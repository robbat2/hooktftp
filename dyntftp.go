package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

const (
	RRQ   uint16 = 1
	WRQ   uint16 = 2
	DATA  uint16 = 3
	ACK   uint16 = 4
	ERROR uint16 = 5
	OACK  uint16 = 6
)

type Connection interface {
	ReadFrom(b []byte) (n int, addr net.Addr, err error)
	Write(p []byte) (n int, err error)
}

type RRQresponse struct {
	conn      Connection
	buffer    []byte
	pos       int
	ack       []byte
	blocksize uint16
	blocknum  uint16
}

func (res *RRQresponse) Write(p []byte) (int, error) {
	out := res.buffer[4:]

	bytecount := res.pos + len(p)

	if bytecount < int(res.blocksize) {
		res.pos += copy(out[res.pos:], p)
		return len(p), nil
	}

	copied := copy(out[res.pos:], p)
	remaining := p[copied:]

	res.pos += copied

	if res.pos == int(res.blocksize) {

		_, err := res.writeBuffer()
		if err != nil {
			return 0, err
		}

		res.pos = 0
		if len(remaining) != 0 {
			return res.Write(remaining)
		}
	}

	return len(p), nil
}

func (res *RRQresponse) writeBuffer() (int, error) {
	res.blocknum++
	binary.BigEndian.PutUint16(res.buffer, DATA)
	binary.BigEndian.PutUint16(res.buffer[2:], res.blocknum)

	out := res.buffer[:res.pos+4]

	written, err := res.conn.Write(out)
	if err != nil {
		fmt.Println("failed to write to connection", err)
		return 0, err
	}

	fmt.Println()
	fmt.Println("waiting for ack:", string(out))
	fmt.Println("raw:", (out))
	fmt.Println("size:", len(out))

	_, _, err = res.conn.ReadFrom(res.ack)
	fmt.Println("got ack", res.ack)
	fmt.Println()

	if err != nil {
		fmt.Println("failed to read ack", err)
		return 0, err
	}

	// TODO: assert ack

	return written, nil

}

func (res *RRQresponse) WriteOACK() error {
	if res.blocksize == 512 {
		return nil
	}

	oackbuffer := make([]byte, 2)
	binary.BigEndian.PutUint16(oackbuffer, OACK)

	oackbuffer = append(oackbuffer, []byte("blksize")...)
	oackbuffer = append(oackbuffer, 0)
	oackbuffer = append(oackbuffer, []byte(strconv.Itoa(res.blocksize))...)
	oackbuffer = append(oackbuffer, 0)

	fmt.Println("oackbuffer", oackbuffer)

	_, err := res.conn.Write(oackbuffer)

	fmt.Println("waiting for oack ack")
	_, _, err = res.conn.ReadFrom(res.ack)
	fmt.Println("got oack", res.ack)

	return err
}

func (res *RRQresponse) End() (int, error) {
	return res.writeBuffer()
}

func NewRRQresponse(clientaddr *net.UDPAddr, blocksize int) (*RRQresponse, error) {

	listenaddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, err
	}

	// fmt.Println("dialing to", clientaddr)
	conn, err := net.DialUDP("udp", listenaddr, clientaddr)
	// conn, err := net.ListenUDP("udp", listenaddr)

	if err != nil {
		fmt.Println("failed to create client conn", err)
		return nil, err
	}

	fmt.Println("listenting client on:", conn.LocalAddr(), conn.RemoteAddr())
	return &RRQresponse{
		conn,
		make([]byte, blocksize+4),
		0,
		make([]byte, 4),
		blocksize,
		0,
	}, nil

}

func SendFile(path string, blocksize int, addr *net.UDPAddr) {

	rrq, err := NewRRQresponse(addr, blocksize)
	if err != nil {
		fmt.Println("Failed to create rrq", err)
		return
	}

	rrq.WriteOACK()
	rrq.Write([]byte("Hello World Foo Bar jea end"))

	rrq.End()

}

func sliceUpToNullByte(p []byte) ([]byte, []byte) {
	for i, b := range p {
		if b == 0 {
			return p[0:i], p[i+1 : len(p)]
		}
	}
	return p, nil
}

func main() {
	fmt.Println("hello")
	addr, err := net.ResolveUDPAddr("udp", ":1234")
	if err != nil {
		fmt.Println(err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println(err)
		return
	}

	data := make([]byte, 50)

	for {
		_, client_addr, err := conn.ReadFrom(data)
		client_udpaddr, err := net.ResolveUDPAddr("udp", client_addr.String())

		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("incoming:", string(data))
		fmt.Println("incoming bin:", data)

		opcode := binary.BigEndian.Uint16(data)

		if opcode == RRQ {
			rest := data[2:len(data)]
			requestpath, rest := sliceUpToNullByte(rest)

			mode, rest := sliceUpToNullByte(rest)
			fmt.Println("mode is", mode)

			option, rest := sliceUpToNullByte(rest)

			blocksize := 512

			if string(option) == "blksize" {
				blksizebytes, _ := sliceUpToNullByte(rest)
				blocksize, err = strconv.Atoi(string(blksizebytes))
				if err != nil {
					fmt.Println("Failed to parse blksize", blksizebytes)
					continue
				}

				fmt.Println("custom block size", blocksize)
			}

			go SendFile(string(requestpath), blocksize, client_udpaddr)
		} else {
			fmt.Println("got something else", data)
		}

	}

}
