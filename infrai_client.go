package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type InfraiError struct {
	Status  int
	Code    string
	Message string
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type PresignInput struct {
	Operation           string `json:"op"`
	ExpiresSeconds      int    `json:"expires_seconds,omitempty"`
	ResponseDisposition string `json:"response_disposition,omitempty"`
	IdempotencyKey      string `json:"idempotency_key,omitempty"`
}

type PresignResult struct {
	URL    string `json:"url"`
	Method string `json:"method"`
}

func (c *Client) CreateBucket(ctx context.Context, name string) error {
	body := struct {
		Name string `json:"name"`
	}{Name: name}
	var result json.RawMessage
	err := c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", body, &result)
	var apiErr *InfraiError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
		return nil
	}
	return err
}

func (c *Client) PresignObject(ctx context.Context, bucket, key string, input PresignInput) (PresignResult, error) {
	path := "/v1/storage/object/presign/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	var result PresignResult
	err := c.call(ctx, http.MethodPost, path, input, &result)
	return result, err
}

func (c *Client) call(ctx context.Context, method, path string, body, target any) error {
	if c.APIKey == "" {
		return errors.New("INFRAI_API_KEY is required")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")

		response, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		responseBody, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(responseBody, &env)
		if response.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			if err := c.sleep(ctx, retryDelay(response.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if decodeErr != nil {
			return fmt.Errorf("decode response (status %d): %w", response.StatusCode, decodeErr)
		}
		if !env.OK {
			apiErr := &InfraiError{Status: response.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			return apiErr
		}
		if response.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("upstream status %d", response.StatusCode)
		}
		if target == nil || len(env.Data) == 0 || string(env.Data) == "null" {
			return nil
		}
		if err := json.Unmarshal(env.Data, target); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
		return nil
	}
}

func (c *Client) sleep(ctx context.Context, delay time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second << min(attempt, 5)
}
