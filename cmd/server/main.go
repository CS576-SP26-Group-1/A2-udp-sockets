package main

import (
	"flag"
	"fmt"
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
	PROTO_DEFAULT           = "udp"
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
func handleUserResponse(ctx ServerRuntimeContext, c *net.UDPConn, reportingChan chan error, buf []byte, addr *net.UDPAddr) {
	// since our buffer is ctx.byteLimit+1, we know that, once we strip the newline, we have byteLimit characters (<=256)
	// remove the automatically added newline character
	message := strings.TrimRight(string(buf), "\n") // apparently Windows sends carriage returns... I don't like accomodating for Windows...

	// perform encoding/decoding on client
	transformedMessage := ctx.transformFunc(message)

	fmt.Printf("message: %s, encoded: %s\n", message, transformedMessage)

	_, writeErr := c.WriteToUDP([]byte(transformedMessage), addr)
	if writeErr != nil {
		reportingChan <- fmt.Errorf("failed to write response: %w", writeErr)
		return
	}

}

// long-lasting function to listen for messages until executation is interrupted
func runServer(srContext ServerRuntimeContext) error {
	addr, addrErr := net.ResolveUDPAddr(srContext.protocol, srContext.listenPort)
	if addrErr != nil {
		return fmt.Errorf("failed to calculate network addr: %w", addrErr)
	}

	conn, err := net.ListenUDP(srContext.protocol, addr)
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
		closeErr := conn.Close() // always close listener before process exits.
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
		buf := make([]byte, srContext.byteLimit)
		_, addr, packetReadErr := conn.ReadFromUDP(buf)
		if packetReadErr != nil {
			reportingChannel <- fmt.Errorf("failed to read client packet: %w", packetReadErr)
		}

		go handleUserResponse(srContext, conn, reportingChannel, buf, addr)
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
