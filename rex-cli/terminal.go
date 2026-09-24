package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

// Winsize matches the C winsize struct
type Winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// GetTerminalSize retrieves the current terminal columns and rows using standard ioctl
func GetTerminalSize(fd int) (int, int, error) {
	var ws Winsize
	_, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if err != 0 {
		return 80, 24, err
	}
	return int(ws.Col), int(ws.Row), nil
}

// MakeTerminalRaw puts the terminal connected to the given file descriptor into raw mode
func MakeTerminalRaw(fd int) (*syscall.Termios, error) {
	termios, err := getTermios(fd)
	if err != nil {
		return nil, err
	}

	oldState := *termios

	// Replicate cfmakeraw behavior
	termios.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	termios.Oflag &^= syscall.OPOST
	termios.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	termios.Cflag &^= syscall.CSIZE | syscall.PARENB
	termios.Cflag |= syscall.CS8
	termios.Cc[syscall.VMIN] = 1
	termios.Cc[syscall.VTIME] = 0

	if err := setTermios(fd, termios); err != nil {
		return nil, err
	}

	return &oldState, nil
}

// RestoreTerminal restores the terminal to a previous state
func RestoreTerminal(fd int, state *syscall.Termios) error {
	return setTermios(fd, state)
}

func getTermios(fd int) (*syscall.Termios, error) {
	var termios syscall.Termios
	_, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&termios)),
	)
	if err != 0 {
		return nil, err
	}
	return &termios, nil
}

func setTermios(fd int, termios *syscall.Termios) error {
	_, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TCSETS),
		uintptr(unsafe.Pointer(termios)),
	)
	if err != 0 {
		return err
	}
	return nil
}

// InteractiveTerminal attaches the local user terminal to a remote PTY session on the server
func InteractiveTerminal(client *Client, streamID uint16) error {
	stdinFd := int(os.Stdin.Fd())

	// Check if stdin is a terminal by attempting to get termios
	oldState, err := MakeTerminalRaw(stdinFd)
	if err != nil {
		return fmt.Errorf("standard input is not a terminal or failed to enter raw mode: %w", err)
	}
	defer RestoreTerminal(stdinFd, oldState)

	// Get initial terminal window dimensions
	width, height, err := GetTerminalSize(stdinFd)
	if err != nil {
		width, height = 80, 24
	}

	// Register stream channel
	ch := client.RegisterStream(streamID)
	defer client.UnregisterStream(streamID)

	// Spawn remote PTY shell with dimensions
	spawnPayload := make([]byte, 4)
	binary.BigEndian.PutUint16(spawnPayload[0:2], uint16(width))
	binary.BigEndian.PutUint16(spawnPayload[2:4], uint16(height))

	if err := client.Send(OpPTYSpawn, streamID, spawnPayload); err != nil {
		return fmt.Errorf("failed to send spawn request: %w", err)
	}

	// Wait for spawn ACK or error
	frame, ok := <-ch
	if !ok || frame.Opcode == OpError {
		errMsg := "unknown spawn error"
		if frame != nil {
			errMsg = string(frame.Payload)
		}
		return fmt.Errorf("remote PTY spawn failed: %s", errMsg)
	}

	// Listen for local terminal resize signals (SIGWINCH)
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	defer signal.Stop(sigwinch)

	go func() {
		for range sigwinch {
			w, h, err := GetTerminalSize(stdinFd)
			if err == nil {
				resizePayload := make([]byte, 4)
				binary.BigEndian.PutUint16(resizePayload[0:2], uint16(w))
				binary.BigEndian.PutUint16(resizePayload[2:4], uint16(h))
				_ = client.Send(OpPTYResize, streamID, resizePayload)
			}
		}
	}()

	// Pipe Server PTY Output -> Local os.Stdout
	errChan := make(chan error, 2)
	go func() {
		for f := range ch {
			if f.Opcode == OpPTYData {
				_, _ = os.Stdout.Write(f.Payload)
			} else if f.Opcode == OpPTYClose {
				errChan <- nil
				return
			}
		}
		errChan <- nil
	}()

	// Pipe Local os.Stdin -> Server PTY Input (OpPTYData)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if sendErr := client.Send(OpPTYData, streamID, buf[:n]); sendErr != nil {
					errChan <- sendErr
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					errChan <- err
				} else {
					errChan <- nil
				}
				return
			}
		}
	}()

	// Wait until session finishes
	<-errChan

	// Notify server PTY close
	_ = client.Send(OpPTYClose, streamID, nil)

	return nil
}
