package app

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/textproto"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	http "github.com/bogdanfinn/fhttp"
)

type upstreamHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type curlImpersonateClient struct {
	binGetter func() (string, error)
	proxyURL  string
}

func newCurlImpersonateClient(proxyURL string, binGetter func() (string, error)) (*curlImpersonateClient, error) {
	return &curlImpersonateClient{binGetter: binGetter, proxyURL: strings.TrimSpace(proxyURL)}, nil
}

func (c *curlImpersonateClient) Do(req *http.Request) (*http.Response, error) {
	bin, err := c.binGetter()
	if err != nil {
		return nil, err
	}
	bodyBytes, err := readRequestBody(req)
	if err != nil {
		return nil, err
	}
	args := []string{
		"--silent",
		"--show-error",
		"--no-progress-meter",
		"--compressed",
		"--http2",
		"--dump-header", "-",
		"--request", req.Method,
	}
	if c.proxyURL != "" {
		args = append(args, "--proxy", c.proxyURL)
	}
	for key, values := range req.Header {
		if strings.EqualFold(key, http.HeaderOrderKey) {
			continue
		}
		for _, value := range values {
			if value == "" {
				continue
			}
			args = append(args, "--header", key+": "+value)
		}
	}
	if len(bodyBytes) > 0 || req.Method == http.MethodPost || req.Method == http.MethodPut || req.Method == http.MethodPatch {
		args = append(args, "--data-binary", "@-")
	}
	args = append(args, req.URL.String())

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		defer stdin.Close()
		if len(bodyBytes) > 0 {
			_, _ = stdin.Write(bodyBytes)
		}
	}()

	reader := bufio.NewReader(stdout)
	statusCode, statusText, header, err := readCurlResponseHeader(reader)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("curl-impersonate failed: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}
	body := &curlResponseBody{reader: reader, cmd: cmd, stderr: stderr}
	return &http.Response{
		Status:     fmt.Sprintf("%d %s", statusCode, statusText),
		StatusCode: statusCode,
		Header:     header,
		Body:       body,
		Request:    req,
	}, nil
}

func readRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	defer req.Body.Close()
	return io.ReadAll(req.Body)
}

func readCurlResponseHeader(reader *bufio.Reader) (int, string, http.Header, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, "", nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToUpper(line), "HTTP/") {
			return 0, "", nil, fmt.Errorf("unexpected response prefix: %q", line)
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return 0, "", nil, fmt.Errorf("bad status line: %q", line)
		}
		code, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, "", nil, err
		}
		statusText := strings.TrimSpace(strings.TrimPrefix(line, parts[0]+" "+parts[1]))
		header := http.Header{}
		for {
			hl, err := reader.ReadString('\n')
			if err != nil {
				return 0, "", nil, err
			}
			hl = strings.TrimRight(hl, "\r\n")
			if hl == "" {
				break
			}
			k, v, ok := strings.Cut(hl, ":")
			if !ok {
				continue
			}
			header.Add(textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(k)), strings.TrimSpace(v))
		}
		// Skip interim 1xx blocks and continue to the real response.
		if code >= 100 && code < 200 && code != 101 {
			continue
		}
		return code, statusText, header, nil
	}
}

type curlResponseBody struct {
	reader *bufio.Reader
	cmd    *exec.Cmd
	stderr *bytes.Buffer
	once   sync.Once
	err    error
}

func (b *curlResponseBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if err == io.EOF {
		waitErr := b.wait()
		if waitErr != nil {
			return n, waitErr
		}
	}
	return n, err
}

func (b *curlResponseBody) Close() error {
	if b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
	}
	return b.wait()
}

func (b *curlResponseBody) wait() error {
	b.once.Do(func() {
		if err := b.cmd.Wait(); err != nil {
			if b.stderr != nil && b.stderr.Len() > 0 {
				b.err = fmt.Errorf("curl-impersonate failed: %w: %s", err, strings.TrimSpace(b.stderr.String()))
			} else {
				b.err = err
			}
		}
	})
	return b.err
}
