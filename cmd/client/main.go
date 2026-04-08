package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

const (
	// default values
	MESSAGE_DEFAULT     = "Hello World"
	BYTE_LIMIT_DEFAULT  = 256
	LISTEN_PORT_DEFAULT = ":8080"
	PROTO_DEFAULT       = "tcp"
	INTERACTIVE_DEFAULT = false
	COLOR_DEFAULT       = true
	// usage flag help (shown with `-h` flag)
	MESSAGE_USAGE     = "Pass a message (ASCII characters) to send to a server for encoding/decoding."
	LISTEN_PORT_USAGE = "Configure port for client to connect to."
	PROTO_USAGE       = "Configure protcool for client to connect with."
	BYTE_LIMIT_USAGE  = "Adjust bytes expected by/from the server."
	INTERACTIVE_USAGE = "Listen continously to user input."
	COLOR_USAGE       = "Add ANSI formatting to the terminal output."
)

type ClientRuntimeContext struct {
	message         string
	protocol        string
	listenPort      string
	byteLimit       int
	interactiveMode bool
	color           bool
}

func sendMessage(crContext ClientRuntimeContext, conn net.Conn) error {
	// push message to remote server

	// the write to the server here needs a `\n` to guarantee that the server
	// bothers to parse it (especially in one-shot that doesn't use newlines)
	// instead of just reading it infinitely
	_, err := fmt.Fprintf(conn, "%s\n", crContext.message)
	if err != nil {
		return fmt.Errorf("failed to write to remote server: %w", err)
	}

	// Prepare to accept the response of the server
	reader := bufio.NewReader(conn)

	// read response from remote server
	// probably should do some reasonable timeout because this will be a problem
	// if the server forgets to add a newline
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	response, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return fmt.Errorf("server response timed out")
		}

		return fmt.Errorf("failed to read server response: %w", err)
	}

	// reset deadline
	conn.SetDeadline(time.Time{})

	// server response
	if crContext.color {
		fmt.Printf("\033[1m\033[33mMessage returned:\033[0m %s", response)
	} else {
		fmt.Printf("Message returned: %s", response)
	}

	return nil
}

func runClient(crContext ClientRuntimeContext) error {
	// implement a TCP client as a Dialer?
	var dialer net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// open connection to remove server
	conn, err := dialer.DialContext(ctx, crContext.protocol, crContext.listenPort)
	if err != nil {
		return fmt.Errorf("failed to connect to remote server: %w", err)
	}

	if crContext.interactiveMode {
		// interactive mode:
		// take over the console, sending content of each line (up to `\n`) to server

		notice := "You are now in interactive mode. Please type your message:"
		if crContext.color {
			fmt.Printf("\033[1m\033[35m%s\033[0m\n", notice)
		} else {
			fmt.Println(notice)
		}

		stdIn := bufio.NewScanner(os.Stdin)

		for stdIn.Scan() {
			// original `-message` contents don't matter, so we override it with the new input
			// and pass it to the sendMessage as usual
			crContext.message = stdIn.Text()
			err := sendMessage(crContext, conn)
			if err != nil {
				return err
			}
		}

		// check if scanner ran into any errors while scanning
		err := stdIn.Err()
		if err != nil {
			return fmt.Errorf("failed reading stdin: %w", err)
		}
	} else {
		// one-shot mode: read in `-message` and send that off to the server
		err := sendMessage(crContext, conn)
		if err != nil {
			return err
		}
	}

	// cleanup TCP connection
	err = conn.Close()
	if err != nil {
		return fmt.Errorf("failed to close connection: %w", err)
	}

	return nil
}

// parse command line arguments, with defaults
func parseArgs() *ClientRuntimeContext {
	message := flag.String("message", MESSAGE_DEFAULT, MESSAGE_USAGE)
	protcol := flag.String("proto", PROTO_DEFAULT, PROTO_USAGE)
	listenPort := flag.String("port", LISTEN_PORT_DEFAULT, LISTEN_PORT_USAGE)
	byteLimit := flag.Int("blimit", BYTE_LIMIT_DEFAULT, BYTE_LIMIT_USAGE)
	interactiveMode := flag.Bool("interactive", INTERACTIVE_DEFAULT, INTERACTIVE_USAGE)
	color := flag.Bool("color", COLOR_DEFAULT, COLOR_USAGE)

	flag.Parse()

	return &ClientRuntimeContext{
		message:         *message,
		protocol:        *protcol,
		listenPort:      *listenPort,
		byteLimit:       *byteLimit,
		interactiveMode: *interactiveMode,
		color:           *color,
	}
}

// oneshot function to send a message with command line to a remote server
func main() {
	ctx := parseArgs()
	err := runClient(*ctx)
	if err != nil {
		err := fmt.Errorf("client message send failed: %w", err)
		log.Fatal(err)
	}
}
