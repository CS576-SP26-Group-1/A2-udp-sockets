package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"tcp-sockets/pkg/transform"
)

// is it fine that this is out of main scope?
const (
	// default values
	BYTE_LIMIT_DEFAULT      = 256
	LISTEN_PORT_DEFAULT     = ":8080"
	PROTO_DEFAULT           = "tcp"
	MAX_DRAIN_COUNT_DEFAULT = 3
	// flag names
	ENCODE_FLAG = "encode"
	DECODE_FLAG = "decode"
	// usage flag help (shown with `-h` flag)
	TRANSFORM_USAGE       = "Configure server to encode or decode the passed message."
	LISTEN_PORT_USAGE     = "Configure port for server to listen on."
	PROTO_USAGE           = "Configure protcool for server to listen to."
	BYTE_LIMIT_USAGE      = "Configure maximum bytes for server to accept."
	MAX_DRAIN_COUNT_USAGE = "Configure amount of times to attempt to clear connection buffer before giving up and disconnecting the client."
)

type ServerRuntimeContext struct {
	transformMode string
	transformFunc func(string) string
	listenPort    string
	protocol      string
	byteLimit     int
	maxDrainCount int
}

// per-message client message handler
func handleUserResponse(ctx ServerRuntimeContext, c net.Conn, reportingChan chan error) {
	defer func() {
		// close the connection cleanly, or if that fails, report it
		closeErr := c.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("failed to close %s connection cleanly: %w", ctx.protocol, closeErr)
			reportingChan <- closeErr // reportingChan wa kyou mou kawaii~!!
			return
		}
	}()

	reader := bufio.NewReaderSize(c, ctx.byteLimit+1) // need +1 for the automatic newline added by the client

	// need to handle client interactive mode, so we read the connection in a loop up to every newline
	for {
		// read stream up to a newline character
		line, readErr := reader.ReadSlice('\n')

		if readErr == nil {
			// since our buffer is ctx.byteLimit+1, we know that, once we strip the newline, we have byteLimit characters (<=256)
			// remove the automatically added newline character
			message := strings.TrimRight(string(line), "\n") // apparently Windows sends carriage returns... I don't like accomodating for Windows...

			// perform encoding/decoding on client
			transformedMessage := ctx.transformFunc(message)

			fmt.Printf("message: %s, encoded: %s\n", message, transformedMessage)

			// apparently we can use fmt.Fprintf instead of net.conn.Write since net.conn is a writer
			// pretty neat....
			// write the encoded (or decoded) response back to the client
			_, readErr = fmt.Fprintf(c, "%s\n", transformedMessage)
			if readErr != nil {
				reportingChan <- fmt.Errorf("failed to write response: %w", readErr)
				return
			}
		}

		if errors.Is(readErr, io.EOF) {
			// EOF is sent by client on disconnect, so it's not quite an error
			// that would be worth reporting
			return
		}

		// if the buffer fills without a newline, we can assume that the message
		// is longer than 256 characters (or blimit)
		if errors.Is(readErr, bufio.ErrBufferFull) {
			// if we had a super long messsage come in that does have a new line,
			// we need this guard clause here to fully drain the message before
			// resetting

			// report a non-fatal error to the server logs and the client if the client sends a message
			// longer than 256 characters
			// since we have a fixed buffer size, we will only ever read byteLimit, so we can only
			// report what we wanted from the client, not how much the client actually sent
			oversizeErr := fmt.Sprintf("bad request from client: message exceeded server's byte limit (wanted: %d)\n", ctx.byteLimit)
			fmt.Print(oversizeErr)
			_, _ = fmt.Fprint(c, oversizeErr)

			isDrained := false
			drainCount := 0

			// attempt to drain the connection of the big message until we give up
			for !isDrained && drainCount < ctx.maxDrainCount { // I can't believe Golang doens't have a while loop keyword...
				_, drainErr := reader.ReadSlice('\n')

				// cases:
				// - no error: found newline, drained successfully
				// - EOF: client disconnected, can't fix this
				// - buffer full: client must be a yapper
				// - some other error: I have no clue what happened. It's probably unrecoverable.
				if drainErr == nil {
					// found the newline, big message was drained
					isDrained = true // in a for-loop, we could break, but I hate break!
				} else if errors.Is(drainErr, io.EOF) {
					// EOF is sent by client on disconnect, so it's not quite an error
					return
				} else if errors.Is(drainErr, bufio.ErrBufferFull) {
					drainCount++
					continue
				} else {
					// whatever error this was, it must've been really bad
					reportingChan <- fmt.Errorf("failed to drain message: %w", drainErr)
					return
				}
			}

			if !isDrained {
				// whatever the client sent must be malformed (no newline) or they're trying to be
				// really annoying with a stupidly large message
				oversizeErr := "disconnected client due to super bad request from client: failed to drain message\n"
				fmt.Print(oversizeErr)
				_, _ = fmt.Fprint(c, oversizeErr)
				return
			}

			// don't want to hit that last clause in this function... might be bad :P
			continue
		}

		if readErr != nil {
			// must've been some fatal error that happened with the reader
			reportingChan <- fmt.Errorf("failed to read from client: %w", readErr)
			return
		}
	}
}

// long-lasting function to listen for messages until executation is interrupted
func runServer(srContext ServerRuntimeContext) error {
	var err error
	listener, err := net.Listen(srContext.protocol, srContext.listenPort)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}

	fmt.Printf("%s server listening on %s with transform directive: [%s]\n", srContext.protocol, srContext.listenPort, srContext.transformMode)

	// chose arbitrary buffer size for formatting goroutine errors
	// not sure if it really matters but I figured 10 should be enough if there's
	// more than one connection that was shunted to a goroutine
	reportingChannel := make(chan error, 10)

	// teardown logic for the listener
	defer func() {
		// I assume the chan will get closed on program exit, along with the goroutines.
		closeErr := listener.Close() // always close listener before process exits.
		if closeErr != nil {
			err = fmt.Errorf("failed to close %s listener: %w", srContext.protocol, closeErr)
		}
	}()

	// consumer for reporting channel to stdout
	go func() {
		for reportedErr := range reportingChannel {
			log.Printf("encountered issue with client request: %s\n", reportedErr)
		}
	}()

	// Loop infinitely for pending connections
	for {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			var opErr *net.OpError
			if errors.As(acceptErr, &opErr) && opErr.Temporary() {
				// don't want to fall over if the error is transient
				// should cover timeouts and what-not (ECONNRESET, ECONNABORTED)
				// technically shouldn't use Temporary() since it's deprecated
				// but I don't know the alternative off the top of my head
				// https://cs.opensource.google/go/go/+/refs/tags/go1.26.0:src/net/net.go;l=552
				log.Printf("non-fatal issue occured during connection accept: %s\n", opErr)
				continue
			} else {
				return fmt.Errorf("fatal issue occured while accepting request: %w", acceptErr)
			}
		}

		// in "net" package example, connection is handled in a
		// concurrent goroutine while the server continues listening
		// for more acceptions. Do we want this in our implementation?
		go handleUserResponse(srContext, conn, reportingChannel)
	}
}

// check if user passed a valid transform directive and return the associated function, else return error
func validateTransform(transformDirective string) (func(string) string, error) {
	switch transformDirective {
	case ENCODE_FLAG:
		return transform.Encode, nil
	case DECODE_FLAG:
		return transform.Decode, nil
	}
	return nil, fmt.Errorf("invalid transform provided: %s (expected: %s, %s)", transformDirective, ENCODE_FLAG, DECODE_FLAG)
}

// parse command line arguments, with defaults
func parseArgs() (*ServerRuntimeContext, error) {
	transformMode := flag.String("transform", ENCODE_FLAG, TRANSFORM_USAGE)
	listenPort := flag.String("port", LISTEN_PORT_DEFAULT, LISTEN_PORT_USAGE)
	protocol := flag.String("proto", PROTO_DEFAULT, PROTO_USAGE)
	byteLimit := flag.Int("blimit", BYTE_LIMIT_DEFAULT, BYTE_LIMIT_USAGE)
	maxDrainCount := flag.Int("maxdraincount", MAX_DRAIN_COUNT_DEFAULT, MAX_DRAIN_COUNT_USAGE)

	flag.Parse()

	// might as well cache the transform function that we will use, since we are here
	// saves us some cycles from needing an if-statement later
	transformFunc, err := validateTransform(*transformMode)
	if err != nil {
		return nil, err
	}

	return &ServerRuntimeContext{
		transformMode: *transformMode,
		transformFunc: transformFunc,
		listenPort:    *listenPort,
		protocol:      *protocol,
		byteLimit:     *byteLimit,
		maxDrainCount: *maxDrainCount,
	}, nil
}

func main() {
	// get parameters for server executation
	context, err := parseArgs()
	if err != nil {
		err = fmt.Errorf("failed to parse flags: %w", err)
		log.Fatal(err)
	}

	// spin up the listener for the server
	err = runServer(*context)
	if err != nil {
		err = fmt.Errorf("%s connection closed due to error: %w", context.protocol, err)
		log.Fatal(err)
	}
}
