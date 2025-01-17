package paypal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"time"

	"golang.org/x/oauth2/clientcredentials"
)

// NewClient returns new Client struct
// APIBase is a base API URL, for testing you can use paypal.APIBaseSandBox
func NewClient(clientID string, secret string, APIBase string) (*Client, error) {
	if clientID == "" || secret == "" || APIBase == "" {
		return nil, errors.New("ClientID, Secret and APIBase are required to create a Client")
	}
	return &Client{
		ccCfg: &clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: secret,
			TokenURL:     APIBase + "/v1/oauth2/token",
		},
		ClientID: clientID,
		Secret:   secret,
		APIBase:  APIBase,
	}, nil
}

// SetAccessToken sets saved token to current client
func (c *Client) SetAccessToken(token string) {
	c.Token = &TokenResponse{
		Token: token,
	}
	c.tokenExpiresAt = time.Time{}
}

// SetLog will set/change the output destination.
// If log file is set paypal will log all requests and responses to this Writer
func (c *Client) SetLog(log io.Writer) {
	c.Log = log
}

// SetReturnRepresentation enables verbose response
// Verbose response: https://developer.paypal.com/docs/api/orders/v2/#orders-authorize-header-parameters
func (c *Client) SetReturnRepresentation() {
	c.returnRepresentation = true
}

// Send makes a request to the API, the response body will be
// unmarshaled into v, or if v is an io.Writer, the response will
// be written to it without decoding
func (c *Client) Send(req *http.Request, v interface{}) error {
	var (
		err  error
		resp *http.Response
		data []byte
	)

	// Set default headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en_US")

	// Default values for headers
	if req.Header.Get("Content-type") == "" {
		req.Header.Set("Content-type", "application/json")
	}
	if c.returnRepresentation {
		req.Header.Set("Prefer", "return=representation")
	}

	// get client
	client := c.ccCfg.Client(req.Context())

	resp, err = client.Do(req)
	c.log(req, resp)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errResp := &ErrorResponse{Response: resp}
		data, err = ioutil.ReadAll(resp.Body)

		if err == nil && len(data) > 0 {
			json.Unmarshal(data, errResp)
		}

		return errResp
	}
	if v == nil {
		return nil
	}

	if w, ok := v.(io.Writer); ok {
		io.Copy(w, resp.Body)
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

// SendWithAuth makes a request to the API and apply OAuth2 header automatically.
// If the access token soon to be expired or already expired, it will try to get a new one before
// making the main request
// client.Token will be updated when changed
func (c *Client) SendWithAuth(req *http.Request, v interface{}) error {
	return c.Send(req, v)
}

// SendWithBasicAuth makes a request to the API using clientID:secret basic auth
func (c *Client) SendWithBasicAuth(req *http.Request, v interface{}) error {
	req.SetBasicAuth(c.ClientID, c.Secret)

	return c.Send(req, v)
}

// NewRequest constructs a request
// Convert payload to a JSON
func (c *Client) NewRequest(ctx context.Context, method, url string, payload interface{}) (*http.Request, error) {
	var buf io.Reader
	if payload != nil {
		b, err := json.Marshal(&payload)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewBuffer(b)
	}
	return http.NewRequestWithContext(ctx, method, url, buf)
}

// log will dump request and response to the log file
func (c *Client) log(r *http.Request, resp *http.Response) {
	if c.Log != nil {
		var (
			reqDump  string
			respDump []byte
		)

		if r != nil {
			reqDump = fmt.Sprintf("%s %s. Data: %s", r.Method, r.URL.String(), r.Form.Encode())
		}
		if resp != nil {
			respDump, _ = httputil.DumpResponse(resp, true)
		}

		c.Log.Write([]byte(fmt.Sprintf("Request: %s\nResponse: %s\n", reqDump, string(respDump))))
	}
}
