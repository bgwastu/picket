package sshstream

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestBridgeCopiesBothDirections(t *testing.T) {
	leftA, leftB := net.Pipe()
	rightA, rightB := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- Bridge(context.Background(), leftA, rightA, time.Second, time.Second) }()
	go func() { _, _ = leftB.Write([]byte("to right")) }()
	buf := make([]byte, 8)
	if _, err := io.ReadFull(rightB, buf); err != nil || string(buf) != "to right" {
		t.Fatalf("right read: %q %v", buf, err)
	}
	go func() { _, _ = rightB.Write([]byte("to left")) }()
	buf = make([]byte, 7)
	if _, err := io.ReadFull(leftB, buf); err != nil || string(buf) != "to left" {
		t.Fatalf("left read: %q %v", buf, err)
	}
	_ = leftB.Close()
	_ = rightB.Close()
	if err := <-done; err != nil {
		t.Fatalf("bridge: %v", err)
	}
}

func TestBridgeIdleTimeoutClosesSockets(t *testing.T) {
	leftA, leftB := net.Pipe()
	rightA, rightB := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- Bridge(context.Background(), leftA, rightA, 20*time.Millisecond, time.Second) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("bridge did not expire")
	}
	_ = leftB.Close()
	_ = rightB.Close()
}
