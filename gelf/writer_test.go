// Copyright 2012 SocialCode. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package gelf

import (
	"compress/flate"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"time"
)

func TestNewWriter(t *testing.T) {
	w, err := NewWriter("", "")
	if err == nil || w != nil {
		t.Errorf("New didn't fail")
		return
	}
}

func sendAndRecv(msgData string, compress CompressType) (*Message, error) {
	r, err := NewReader("127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("NewReader: %s", err)
	}

	w, err := NewWriter(r.Addr(), "")
	if err != nil {
		return nil, fmt.Errorf("NewWriter: %s", err)
	}
	w.CompressionType = compress

	if _, err = w.Write([]byte(msgData)); err != nil {
		return nil, fmt.Errorf("w.Write: %s", err)
	}

	return r.ReadMessage()
}

func sendAndRecvMsg(msg *Message, compress CompressType) (*Message, error) {
	r, err := NewReader("127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("NewReader: %s", err)
	}

	w, err := NewWriter(r.Addr(), "")
	if err != nil {
		return nil, fmt.Errorf("NewWriter: %s", err)
	}
	w.CompressionType = compress

	if err = w.WriteMessage(msg); err != nil {
		return nil, fmt.Errorf("w.Write: %s", err)
	}

	return r.ReadMessage()
}

// tests single-message (non-chunked) messages that are split over
// multiple lines
func TestWriteSmallMultiLine(t *testing.T) {
	for _, i := range []CompressType{CompressGzip, CompressZlib, CompressNone} {
		msgData := "awesomesauce\nbananas"

		msg, err := sendAndRecv(msgData, i)
		if err != nil {
			t.Errorf("sendAndRecv: %s", err)
			return
		}

		if msg.Short != "awesomesauce" {
			t.Errorf("msg.Short: expected %s, got %s", msgData, msg.Full)
			return
		}

		if msg.Full != msgData {
			t.Errorf("msg.Full: expected %s, got %s", msgData, msg.Full)
			return
		}
	}
}

func TestGetCaller(t *testing.T) {
	file, line := getCallerIgnoringLogMulti(1000)
	if line != 0 || file != "???" {
		t.Errorf("didn't fail 1 %s %d", file, line)
		return
	}

	file, _ = getCaller(0)
	if !strings.HasSuffix(file, "/gelf/writer_test.go") {
		t.Errorf("not writer_test.go 1? %s", file)
	}

	file, _ = getCallerIgnoringLogMulti(0)
	if !strings.HasSuffix(file, "/gelf/writer_test.go") {
		t.Errorf("not writer_test.go 2? %s", file)
	}
}

// tests single-message (chunked) messages
func TestWriteBigChunked(t *testing.T) {
	randData := make([]byte, 4096)
	if _, err := rand.Read(randData); err != nil {
		t.Errorf("cannot get random data: %s", err)
		return
	}
	msgData := "awesomesauce\n" + base64.StdEncoding.EncodeToString(randData)

	for _, i := range []CompressType{CompressGzip, CompressZlib} {
		msg, err := sendAndRecv(msgData, i)
		if err != nil {
			t.Errorf("sendAndRecv: %s", err)
			return
		}

		if msg.Short != "awesomesauce" {
			t.Errorf("msg.Short: expected %s, got %s", msgData, msg.Full)
			return
		}

		if msg.Full != msgData {
			t.Errorf("msg.Full: expected %s, got %s", msgData, msg.Full)
			return
		}
	}
}

// tests messages with extra data
func TestExtraData(t *testing.T) {

	// time.Now().Unix() seems fine, UnixNano() won't roundtrip
	// through string -> float64 -> int64
	extra := map[string]interface{}{
		"_a":    10 * time.Now().Unix(),
		"C":     9,
		"_file": "writer_test.go",
		"_line": 186,
	}

	short := "quick"
	full := short + "\nwith more detail"
	m := Message{
		Version:  "1.0",
		Host:     "fake-host",
		Short:    string(short),
		Full:     string(full),
		TimeUnix: float64(time.Now().Unix()),
		Level:    6, // info
		Facility: "writer_test",
		Extra:    extra,
		RawExtra: []byte(`{"woo": "hoo"}`),
	}

	for _, i := range []CompressType{CompressGzip, CompressZlib} {
		msg, err := sendAndRecvMsg(&m, i)
		if err != nil {
			t.Errorf("sendAndRecv: %s", err)
			return
		}

		if msg.Short != short {
			t.Errorf("msg.Short: expected %s, got %s", short, msg.Full)
			return
		}

		if msg.Full != full {
			t.Errorf("msg.Full: expected %s, got %s", full, msg.Full)
			return
		}

		if len(msg.Extra) != 3 {
			t.Errorf("extra extra fields in %v", msg.Extra)
			return
		}

		if int64(msg.Extra["_a"].(float64)) != extra["_a"].(int64) {
			t.Errorf("_a didn't roundtrip (%v != %v)", int64(msg.Extra["_a"].(float64)), extra["_a"].(int64))
			return
		}

		if string(msg.Extra["_file"].(string)) != extra["_file"] {
			t.Errorf("_file didn't roundtrip (%v != %v)", msg.Extra["_file"].(string), extra["_file"].(string))
			return
		}

		if int(msg.Extra["_line"].(float64)) != extra["_line"].(int) {
			t.Errorf("_line didn't roundtrip (%v != %v)", int(msg.Extra["_line"].(float64)), extra["_line"].(int))
			return
		}
	}
}

func BenchmarkWriteBestSpeed(b *testing.B) {
	r, err := NewReader("127.0.0.1:0")
	if err != nil {
		b.Fatalf("NewReader: %s", err)
	}
	go io.Copy(ioutil.Discard, r)
	w, err := NewWriter(r.Addr(), "")
	if err != nil {
		b.Fatalf("NewWriter: %s", err)
	}
	w.CompressionLevel = flate.BestSpeed
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.WriteMessage(&Message{
			Version:  "1.1",
			Host:     w.hostname,
			Short:    "short message",
			Full:     "full message",
			TimeUnix: float64(time.Now().Unix()),
			Level:    6, // info
			Facility: w.Facility,
			Extra:    map[string]interface{}{"_file": "1234", "_line": "3456"},
		})
	}
}

func BenchmarkWriteNoCompression(b *testing.B) {
	r, err := NewReader("127.0.0.1:0")
	if err != nil {
		b.Fatalf("NewReader: %s", err)
	}
	go io.Copy(ioutil.Discard, r)
	w, err := NewWriter(r.Addr(), "")
	if err != nil {
		b.Fatalf("NewWriter: %s", err)
	}
	w.CompressionLevel = flate.NoCompression
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.WriteMessage(&Message{
			Version:  "1.1",
			Host:     w.hostname,
			Short:    "short message",
			Full:     "full message",
			TimeUnix: float64(time.Now().Unix()),
			Level:    6, // info
			Facility: w.Facility,
			Extra:    map[string]interface{}{"_file": "1234", "_line": "3456"},
		})
	}
}

func BenchmarkWriteDisableCompressionCompletely(b *testing.B) {
	r, err := NewReader("127.0.0.1:0")
	if err != nil {
		b.Fatalf("NewReader: %s", err)
	}
	go io.Copy(ioutil.Discard, r)
	w, err := NewWriter(r.Addr(), "")
	if err != nil {
		b.Fatalf("NewWriter: %s", err)
	}
	w.CompressionType = CompressNone
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.WriteMessage(&Message{
			Version:  "1.1",
			Host:     w.hostname,
			Short:    "short message",
			Full:     "full message",
			TimeUnix: float64(time.Now().Unix()),
			Level:    6, // info
			Facility: w.Facility,
			Extra:    map[string]interface{}{"_file": "1234", "_line": "3456"},
		})
	}
}

func BenchmarkWriteDisableCompressionAndPreencodeExtra(b *testing.B) {
	r, err := NewReader("127.0.0.1:0")
	if err != nil {
		b.Fatalf("NewReader: %s", err)
	}
	go io.Copy(ioutil.Discard, r)
	w, err := NewWriter(r.Addr(), "")
	if err != nil {
		b.Fatalf("NewWriter: %s", err)
	}
	w.CompressionType = CompressNone
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.WriteMessage(&Message{
			Version:  "1.1",
			Host:     w.hostname,
			Short:    "short message",
			Full:     "full message",
			TimeUnix: float64(time.Now().Unix()),
			Level:    6, // info
			Facility: w.Facility,
			RawExtra: json.RawMessage(`{"_file":"1234","_line": "3456"}`),
		})
	}
}
