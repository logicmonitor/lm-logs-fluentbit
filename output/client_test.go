package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

//	"github.com/fluent/fluent-bit-go/output"
	"github.com/stretchr/testify/require"
)

const (
	lmTestToken = "123456789"
)

func TestClientWithDefaults(test *testing.T) {
	lmClient, err := NewClient(lmTestToken)
	require.NoError(test, err)
	require.Equal(test, lmClient.listenerURL, defaultURL)
	require.Equal(test, lmClient.logger.debug, false)
}

func TestRequestHeaders(test *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(test, "gzip", r.Header.Get("Content-Encoding"))
		require.Equal(test, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	lmClient := lmTestClient(testServer.URL)
	res := lmClient.Send([]byte("test"))
	require.Equal(test, res, output.FLB_OK)
}

func TestStatusOKCodeResponse(test *testing.T) {
	doStatusCodeResponseTest(test, http.StatusOK, output.FLB_OK)
}

func Test1xxStatusCodeResponse(test *testing.T) {
	doStatusCodeResponseTest(test, http.StatusSwitchingProtocols, output.FLB_ERROR)
}

func Test3xxStatusCodeResponse(test *testing.T) {
	doStatusCodeResponseTest(test, http.StatusMultipleChoices, output.FLB_ERROR)
}

func Test4xxStatusCodeResponse(test *testing.T) {
	doStatusCodeResponseTest(test, http.StatusForbidden, output.FLB_ERROR)
}

func Test5xxStatusCodeResponse(test *testing.T) {
	doStatusCodeResponseTest(test, http.StatusInternalServerError, output.FLB_RETRY)
}

func TestBulkSending(test *testing.T) {
	bulks := 3
	testServer := httptest.NewServer(http.NotFoundHandler())
	testServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logs, err := readLogs(r)
		require.NoError(test, err)
		require.Len(test, logs, 1) // each log is one bulk
		w.WriteHeader(http.StatusOK)
	})
	defer testServer.Close()

	lmClient, err := NewClient(lmTestToken,
		SetURL(testServer.URL),
		SetDebug(true),
		SetBodySizeThreshold(0),
	)
	require.NoError(test, err)

	for i := 1; i <= bulks; i++ {
		msg := fmt.Sprintf("bulk - %d", i)
		ok := lmClient.Send([]byte(msg))
		require.Equal(test, ok, output.FLB_OK)
	}

	lmClient.Flush()
}

func readLogs(r *http.Request) ([]string, error) {
	gzipReader, err := gzip.NewReader(r.Body)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(gzipReader)

	logs := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		logs = append(logs, line)
	}

	return logs, nil
}

func doStatusCodeResponseTest(test *testing.T, statusCode int, expected int) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode == http.StatusOK {
			w.WriteHeader(statusCode)
			return
		}
		http.Error(w, "error!!!", statusCode)
	}))
	defer testServer.Close()

	testURL, err := url.Parse(fmt.Sprintf("http://%s", testServer.Listener.Addr().String()))
	require.NoError(test, err)

	lmClient := lmTestClient(testURL.String())
	lmClient.Send([]byte("test"))
	res := lmClient.Flush()
	require.Equal(test, res, expected)
}

func lmTestClient(testURL string) *lmClient {
	return &lmClient{
		token:                lmTestToken,
		listenerURL:          testURL,
		logger:               NewLogger("testing", true),
		client:               http.DefaultClient,
		sizeThresholdInBytes: maxRequestBodySizeInBytes,
	}
}
