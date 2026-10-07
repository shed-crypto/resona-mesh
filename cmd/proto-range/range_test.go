package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAudioRangeHandler(t *testing.T) {
	// 1 MiB test buffer
	const totalSize = 1024 * 1024
	testData := make([]byte, totalSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	handler := AudioRangeHandler(testData)

	t.Run("Full GET request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/audio", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
		if res.Header.Get("Accept-Ranges") != "bytes" {
			t.Fatalf("expected Accept-Ranges: bytes, got %s", res.Header.Get("Accept-Ranges"))
		}
		body, _ := io.ReadAll(res.Body)
		if len(body) != totalSize {
			t.Fatalf("expected %d bytes, got %d", totalSize, len(body))
		}
	})

	t.Run("HEAD request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodHead, "/audio", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", res.StatusCode)
		}
		body, _ := io.ReadAll(res.Body)
		if len(body) != 0 {
			t.Fatalf("expected empty body for HEAD, got %d bytes", len(body))
		}
	})

	t.Run("Partial Range request bytes=0-1023", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/audio", nil)
		req.Header.Set("Range", "bytes=0-1023")
		rec := httptest.NewRecorder()
		handler(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		if res.StatusCode != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content, got %d", res.StatusCode)
		}
		expectedCR := "bytes 0-1023/1048576"
		if res.Header.Get("Content-Range") != expectedCR {
			t.Fatalf("expected Content-Range %s, got %s", expectedCR, res.Header.Get("Content-Range"))
		}

		body, _ := io.ReadAll(res.Body)
		if len(body) != 1024 {
			t.Fatalf("expected 1024 bytes, got %d", len(body))
		}
		if !bytes.Equal(body, testData[0:1024]) {
			t.Fatal("range payload bytes mismatch")
		}
	})

	t.Run("Open-ended Range bytes=524288-", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/audio", nil)
		req.Header.Set("Range", "bytes=524288-")
		rec := httptest.NewRecorder()
		handler(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		if res.StatusCode != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content, got %d", res.StatusCode)
		}
		expectedCR := "bytes 524288-1048575/1048576"
		if res.Header.Get("Content-Range") != expectedCR {
			t.Fatalf("expected Content-Range %s, got %s", expectedCR, res.Header.Get("Content-Range"))
		}

		body, _ := io.ReadAll(res.Body)
		if len(body) != 524288 {
			t.Fatalf("expected 524288 bytes, got %d", len(body))
		}
		if !bytes.Equal(body, testData[524288:]) {
			t.Fatal("open-ended range payload mismatch")
		}
	})

	t.Run("Suffix Range bytes=-512", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/audio", nil)
		req.Header.Set("Range", "bytes=-512")
		rec := httptest.NewRecorder()
		handler(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		if res.StatusCode != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content, got %d", res.StatusCode)
		}
		expectedCR := "bytes 1048064-1048575/1048576"
		if res.Header.Get("Content-Range") != expectedCR {
			t.Fatalf("expected Content-Range %s, got %s", expectedCR, res.Header.Get("Content-Range"))
		}

		body, _ := io.ReadAll(res.Body)
		if len(body) != 512 {
			t.Fatalf("expected 512 bytes, got %d", len(body))
		}
		if !bytes.Equal(body, testData[totalSize-512:]) {
			t.Fatal("suffix range payload mismatch")
		}
	})

	t.Run("Invalid out-of-bounds Range", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/audio", nil)
		req.Header.Set("Range", "bytes=2000000-3000000")
		rec := httptest.NewRecorder()
		handler(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		if res.StatusCode != http.StatusRequestedRangeNotSatisfiable {
			t.Fatalf("expected 416 Range Not Satisfiable, got %d", res.StatusCode)
		}
	})
}
