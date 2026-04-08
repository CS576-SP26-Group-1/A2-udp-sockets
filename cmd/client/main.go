package main

import (
	"bufio"
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
	BYTE_LIMIT_DEFAULT  = 4096
	LISTEN_PORT_DEFAULT = ":0"
	SEND_PORT_DEFAULT   = ":8080"
	PROTO_DEFAULT       = "udp"
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
	sendPort        string
	listenPort      string
	byteLimit       int
	interactiveMode bool
	color           bool
}

func sendMessage(crContext ClientRuntimeContext, conn *net.UDPConn, addr *net.UDPAddr) error {
	// push message to remote server
	_, err := conn.WriteToUDP([]byte(crContext.message), addr)
	if err != nil {
		return fmt.Errorf("failed to write to remote server: %w", err)
	}

	// read response from remote server
	// probably should do some reasonable timeout because this will be a problem
	// if the server forgets to add a newline
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// Prepare to accept the response of the server
	buf := make([]byte, crContext.byteLimit)
	_, _, err = conn.ReadFromUDP(buf)

	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return fmt.Errorf("server response timed out")
		}

		return fmt.Errorf("failed to read server response: %w", err)
	}

	response := string(buf)

	// reset deadline
	conn.SetDeadline(time.Time{})

	// server response
	if crContext.color {
		fmt.Printf("\033[1m\033[33mMessage returned:\033[0m %s\n", response)
	} else {
		fmt.Printf("Message returned: %s\n", response)
	}

	return nil
}

func runClient(crContext ClientRuntimeContext) error {
	homeAddr, addrErr := net.ResolveUDPAddr(crContext.protocol, crContext.listenPort)
	if addrErr != nil {
		return fmt.Errorf("failed to calculate local network addr: %w", addrErr)
	}

	destAddr, addrErr := net.ResolveUDPAddr(crContext.protocol, crContext.sendPort)
	if addrErr != nil {
		return fmt.Errorf("failed to calculate destination network addr: %w", addrErr)
	}

	conn, listenErr := net.ListenUDP(crContext.protocol, homeAddr)
	if listenErr != nil {
		return fmt.Errorf("failed to open UDP socket: %w", addrErr)
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
			err := sendMessage(crContext, conn, destAddr)
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
		err := sendMessage(crContext, conn, destAddr)
		if err != nil {
			return err
		}
	}

	// cleanup UDP connection
	err := conn.Close()
	if err != nil {
		return fmt.Errorf("failed to close UDP socket: %w", err)
	}

	return nil
}

// parse command line arguments, with defaults
func parseArgs() *ClientRuntimeContext {
	message := flag.String("message", MESSAGE_DEFAULT, MESSAGE_USAGE)
	protcol := flag.String("proto", PROTO_DEFAULT, PROTO_USAGE)
	listenPort := flag.String("lport", LISTEN_PORT_DEFAULT, LISTEN_PORT_USAGE)
	sendPort := flag.String("dport", SEND_PORT_DEFAULT, LISTEN_PORT_USAGE)
	byteLimit := flag.Int("blimit", BYTE_LIMIT_DEFAULT, BYTE_LIMIT_USAGE)
	interactiveMode := flag.Bool("interactive", INTERACTIVE_DEFAULT, INTERACTIVE_USAGE)
	color := flag.Bool("color", COLOR_DEFAULT, COLOR_USAGE)

	flag.Parse()

	return &ClientRuntimeContext{
		message:         *message,
		protocol:        *protcol,
		listenPort:      *listenPort,
		sendPort:        *sendPort,
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
