# CS576  - UDP Sockets (Programming Assignment 2)

UDP client and server pair for CS 576: Networks & Distributed Systems Group Programming Project 2.

**TODOS:**

---

## Program Specifications

We are to write a UDP *connectionless* client-server pair.
- Again, implement in a language of our choice. (Go again?)

## Server
- Server must receive a UDP message from the client.
- Server starts and runs on a port number of choice.
- Server, upon receiving message, will return the original message contents with a 
  humorous message as ASCII string; done so back over the port of choice. (different or same port?)


## Client
- Client must be able to send a message to the server over UDP.
- Client must be able to receive the message from the UDP server once it has been returned.
- Client is responsible for printing the modified string to the monitor.
