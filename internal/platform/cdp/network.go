package cdp

import (
	"encoding/json"
	"fmt"
)

// This file holds the subset of the Network domain Lens decodes. React Native
// implements it natively since 0.83; like runtime.go it is written by hand and
// decodes leniently, so fields a newer React Native adds are ignored.

// Network method and event names used by the network feature.
const (
	MethodNetworkEnable             = "Network.enable"
	MethodNetworkDisable            = "Network.disable"
	MethodNetworkGetResponseBody    = "Network.getResponseBody"
	MethodNetworkGetRequestPostData = "Network.getRequestPostData"
	EventNetworkRequestWillBeSent   = "Network.requestWillBeSent"
	EventNetworkResponseReceived    = "Network.responseReceived"
	EventNetworkDataReceived        = "Network.dataReceived"
	EventNetworkLoadingFinished     = "Network.loadingFinished"
	EventNetworkLoadingFailed       = "Network.loadingFailed"
)

// Headers mirrors Network.Headers. CDP declares it as a JSON object whose
// values are strings, but implementations occasionally send numbers or
// booleans; those are kept as their JSON text instead of failing the event.
type Headers map[string]string

// UnmarshalJSON decodes a header object, stringifying non-string values.
func (h *Headers) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("cdp: headers: %w", err)
	}
	out := make(Headers, len(raw))
	for k, v := range raw {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[k] = s
			continue
		}
		out[k] = string(v)
	}
	*h = out
	return nil
}

// NetworkRequest mirrors Network.Request.
type NetworkRequest struct {
	URL         string  `json:"url"`
	URLFragment string  `json:"urlFragment,omitempty"`
	Method      string  `json:"method"`
	Headers     Headers `json:"headers"`
	PostData    string  `json:"postData,omitempty"`
	HasPostData bool    `json:"hasPostData,omitempty"`
}

// ResourceTiming mirrors Network.ResourceTiming. Every field after
// RequestTime is milliseconds relative to RequestTime (seconds); -1 means the
// phase did not happen.
type ResourceTiming struct {
	RequestTime       float64 `json:"requestTime"`
	ProxyStart        float64 `json:"proxyStart"`
	ProxyEnd          float64 `json:"proxyEnd"`
	DNSStart          float64 `json:"dnsStart"`
	DNSEnd            float64 `json:"dnsEnd"`
	ConnectStart      float64 `json:"connectStart"`
	ConnectEnd        float64 `json:"connectEnd"`
	SSLStart          float64 `json:"sslStart"`
	SSLEnd            float64 `json:"sslEnd"`
	SendStart         float64 `json:"sendStart"`
	SendEnd           float64 `json:"sendEnd"`
	ReceiveHeadersEnd float64 `json:"receiveHeadersEnd"`
}

// NetworkResponse mirrors Network.Response.
type NetworkResponse struct {
	URL               string          `json:"url"`
	Status            int             `json:"status"`
	StatusText        string          `json:"statusText"`
	Headers           Headers         `json:"headers"`
	MimeType          string          `json:"mimeType"`
	RemoteIPAddress   string          `json:"remoteIPAddress,omitempty"`
	RemotePort        int             `json:"remotePort,omitempty"`
	FromDiskCache     bool            `json:"fromDiskCache,omitempty"`
	EncodedDataLength float64         `json:"encodedDataLength,omitempty"`
	Timing            *ResourceTiming `json:"timing,omitempty"`
	Protocol          string          `json:"protocol,omitempty"`
}

// Initiator mirrors Network.Initiator. The stack is kept raw because the
// panel shows it as-is and Hermes frames vary between releases.
type Initiator struct {
	Type       string          `json:"type"`
	URL        string          `json:"url,omitempty"`
	LineNumber *float64        `json:"lineNumber,omitempty"`
	Stack      json.RawMessage `json:"stack,omitempty"`
}

// RequestWillBeSentParams mirrors Network.requestWillBeSent. Timestamp is
// monotonic seconds; WallTime is seconds since the Unix epoch.
type RequestWillBeSentParams struct {
	RequestID        string           `json:"requestId"`
	Request          NetworkRequest   `json:"request"`
	Timestamp        float64          `json:"timestamp"`
	WallTime         float64          `json:"wallTime"`
	Initiator        *Initiator       `json:"initiator,omitempty"`
	RedirectResponse *NetworkResponse `json:"redirectResponse,omitempty"`
	Type             string           `json:"type,omitempty"`
}

// ResponseReceivedParams mirrors Network.responseReceived.
type ResponseReceivedParams struct {
	RequestID string          `json:"requestId"`
	Timestamp float64         `json:"timestamp"`
	Type      string          `json:"type,omitempty"`
	Response  NetworkResponse `json:"response"`
}

// DataReceivedParams mirrors Network.dataReceived.
type DataReceivedParams struct {
	RequestID         string  `json:"requestId"`
	Timestamp         float64 `json:"timestamp"`
	DataLength        float64 `json:"dataLength"`
	EncodedDataLength float64 `json:"encodedDataLength"`
}

// LoadingFinishedParams mirrors Network.loadingFinished.
type LoadingFinishedParams struct {
	RequestID         string  `json:"requestId"`
	Timestamp         float64 `json:"timestamp"`
	EncodedDataLength float64 `json:"encodedDataLength"`
}

// LoadingFailedParams mirrors Network.loadingFailed.
type LoadingFailedParams struct {
	RequestID string  `json:"requestId"`
	Timestamp float64 `json:"timestamp"`
	Type      string  `json:"type,omitempty"`
	ErrorText string  `json:"errorText"`
	Canceled  bool    `json:"canceled,omitempty"`
}

// GetResponseBodyParams is the request of Network.getResponseBody.
type GetResponseBodyParams struct {
	RequestID string `json:"requestId"`
}

// GetResponseBodyResult is the reply of Network.getResponseBody.
type GetResponseBodyResult struct {
	Body          string `json:"body"`
	Base64Encoded bool   `json:"base64Encoded"`
}

// GetRequestPostDataParams is the request of Network.getRequestPostData.
type GetRequestPostDataParams struct {
	RequestID string `json:"requestId"`
}

// GetRequestPostDataResult is the reply of Network.getRequestPostData.
type GetRequestPostDataResult struct {
	PostData string `json:"postData"`
}
