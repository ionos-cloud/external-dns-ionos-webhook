package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider/webhook/api"

	"github.com/ionos-cloud/external-dns-ionos-webhook/cmd/webhook/init/configuration"
)

type testCase struct {
	name                      string
	returnRecords             []*endpoint.Endpoint
	returnAdjustedEndpoints   []*endpoint.Endpoint
	returnDomainFilter        endpoint.DomainFilter
	hasError                  error
	method                    string
	path                      string
	headers                   map[string]string
	body                      string
	expectedStatusCode        int
	expectedResponseHeaders   map[string]string
	expectedBody              string
	expectedChanges           *plan.Changes
	expectedEndpointsToAdjust []*endpoint.Endpoint
	log.Ext1FieldLogger
}

var (
	mockProvider *MockProvider
	config       configuration.Config
)

func TestMain(m *testing.M) {
	mockProvider = &MockProvider{}
	config = configuration.Init()
	srv := Init(config, api.WebhookServer{Provider: mockProvider})
	go ShutdownGracefully(srv)
	time.Sleep(300 * time.Millisecond)
	m.Run()
	if err := srv.Shutdown(context.TODO()); err != nil {
		panic(err)
	}
}

func TestRecords(t *testing.T) {
	testCases := []testCase{
		{
			name: "happy case",
			returnRecords: []*endpoint.Endpoint{
				{
					DNSName:    "test.example.com",
					Targets:    []string{""},
					RecordType: "A",
					RecordTTL:  3600,
					Labels: map[string]string{
						"label1": "value1",
					},
				},
			},
			method:             http.MethodGet,
			headers:            map[string]string{"Accept": "application/external.dns.webhook+json;version=1"},
			path:               "/records",
			body:               "",
			expectedStatusCode: http.StatusOK,
			expectedResponseHeaders: map[string]string{
				"Content-Type": api.MediaTypeFormatAndVersion,
			},
			expectedBody: "[{\"dnsName\":\"test.example.com\",\"targets\":[\"\"],\"recordType\":\"A\",\"recordTTL\":3600,\"labels\":{\"label1\":\"value1\"}}]",
		},
		/* 		{
		   			name:               "no accept header",
		   			method:             http.MethodGet,
		   			headers:            map[string]string{},
		   			path:               "/records",
		   			body:               "",
		   			expectedStatusCode: http.StatusNotAcceptable,
		   			expectedResponseHeaders: map[string]string{
		   				"Content-Type": "text/plain",
		   			},
		   			expectedBody: "client must provide an accept header",
		   		},
		   		{
		   			name:               "wrong accept header",
		   			method:             http.MethodGet,
		   			headers:            map[string]string{"Accept": "invalid"},
		   			path:               "/records",
		   			body:               "",
		   			expectedStatusCode: http.StatusUnsupportedMediaType,
		   			expectedResponseHeaders: map[string]string{
		   				"Content-Type": "text/plain",
		   			},
		   			expectedBody: "client must provide a valid versioned media type in the accept header: unsupported media type version: 'invalid'. Supported media types are: 'application/external.dns.webhook+json;version=1'",
		   		}, */
		{
			name:               "backend error",
			hasError:           fmt.Errorf("backend error"),
			method:             http.MethodGet,
			path:               "/records",
			body:               "",
			expectedStatusCode: http.StatusInternalServerError,
		},
	}
	executeTestCases(t, testCases)
}

func TestApplyChanges(t *testing.T) {
	testCases := []testCase{
		{
			name:   "happy case",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Type": "application/external.dns.webhook+json;version=1",
			},
			path: "/records",
			body: `
{
    "Create": [
        {
            "dnsName": "test.example.com",
            "targets": ["11.11.11.11"],
            "recordType": "A",
            "recordTTL": 3600,
            "labels": {
                "label1": "value1",
                "label2": "value2"
            }
        }
    ]
}`,
			expectedStatusCode:      http.StatusNoContent,
			expectedResponseHeaders: map[string]string{},
			expectedBody:            "",
			expectedChanges: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "test.example.com",
						Targets:    []string{"11.11.11.11"},
						RecordType: "A",
						RecordTTL:  3600,
						Labels: map[string]string{
							"label1": "value1",
							"label2": "value2",
						},
					},
				},
			},
		},
		{
			name:   "invalid json",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Type": api.MediaTypeFormatAndVersion,
			},
			path:               "/records",
			body:               "invalid",
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:     "backend error",
			hasError: fmt.Errorf("backend error"),
			method:   http.MethodPost,
			headers: map[string]string{
				"Content-Type": api.MediaTypeFormatAndVersion,
			},
			path: "/records",
			body: `
{
    "Create": [
        {
            "dnsName": "test.example.com",
            "targets": ["11.11.11.11"],
            "recordType": "A",
            "recordTTL": 3600,
            "labels": {
                "label1": "value1",
                "label2": "value2"
            }
        }
    ]
}`,
			expectedStatusCode: http.StatusInternalServerError,
		},
	}
	executeTestCases(t, testCases)
}

func TestAdjustEndpoints(t *testing.T) {
	testCases := []testCase{
		{
			name: "happy case",
			returnAdjustedEndpoints: []*endpoint.Endpoint{
				{
					DNSName:    "adjusted.example.com",
					Targets:    []string{""},
					RecordType: "A",
					RecordTTL:  3600,
					Labels: map[string]string{
						"label1": "value1",
					},
				},
			},
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Type": "application/external.dns.webhook+json;version=1",
				"Accept":       "application/external.dns.webhook+json;version=1",
			},
			path: "/adjustendpoints",
			body: `
[
	{
		"dnsName": "toadjust.example.com",
		"targets": [],
		"recordType": "A",
		"recordTTL": 3600,
		"labels": {
			"label1": "value1",
			"label2": "value2"
		}
	}
]`,
			expectedStatusCode: http.StatusOK,
			expectedResponseHeaders: map[string]string{
				"Content-Type": "application/external.dns.webhook+json;version=1",
			},
			expectedBody: "[{\"dnsName\":\"adjusted.example.com\",\"targets\":[\"\"],\"recordType\":\"A\",\"recordTTL\":3600,\"labels\":{\"label1\":\"value1\"}}]",
			expectedEndpointsToAdjust: []*endpoint.Endpoint{
				{
					DNSName:    "toadjust.example.com",
					Targets:    []string{},
					RecordType: "A",
					RecordTTL:  3600,
					Labels: map[string]string{
						"label1": "value1",
						"label2": "value2",
					},
				},
			},
		},
		{
			name:   "invalid json",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Type": api.MediaTypeFormatAndVersion,
			},
			path:               "/adjustendpoints",
			body:               "invalid",
			expectedStatusCode: http.StatusBadRequest,
		},
	}
	executeTestCases(t, testCases)
}

func TestNegotiate(t *testing.T) {
	testCases := []testCase{
		{
			name:               "happy case",
			returnDomainFilter: endpoint.NewDomainFilter([]string{"a.de"}),
			method:             http.MethodGet,
			headers:            map[string]string{"Accept": "application/external.dns.webhook+json;version=1"},
			path:               "/",
			body:               "",
			expectedStatusCode: http.StatusOK,
			expectedResponseHeaders: map[string]string{
				"Content-Type": "application/external.dns.webhook+json;version=1",
			},
			expectedBody: `{"include":["a.de"]}`,
		},
	}
	executeTestCases(t, testCases)
}

func TestMetricsServer(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/metrics", config.MetricsHost, config.MetricsPort), nil)
	assert.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	assert.NoError(t, err)

	assert.Equal(t, response.StatusCode, http.StatusOK)
	assert.Contains(t, response.Header.Get("Content-Type"), "text/plain")
	res, err := io.ReadAll(response.Body)
	assert.NoError(t, err)

	body := string(res)

	assert.Contains(t, body, fmt.Sprintf(`go_info{version="%s"}`, runtime.Version()))
}

func executeTestCases(t *testing.T, testCases []testCase) {
	log.SetLevel(log.DebugLevel)
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d. %s", i+1, tc.name), func(t *testing.T) {
			mockProvider.testCase = tc
			mockProvider.t = t
			var bodyReader io.Reader
			if tc.body != "" {
				bodyReader = strings.NewReader(tc.body)
			}
			request, err := http.NewRequest(tc.method, "http://localhost:8888"+tc.path, bodyReader)
			if err != nil {
				t.Error(err)
			}
			for k, v := range tc.headers {
				request.Header.Set(k, v)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Error(err)
			}
			if response.StatusCode != tc.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tc.expectedStatusCode, response.StatusCode)
			}
			for k, v := range tc.expectedResponseHeaders {
				if response.Header.Get(k) != v {
					t.Errorf("expected header '%s' with value '%s', got '%s'", k, v, response.Header.Get(k))
				}
			}
			if tc.expectedBody != "" {
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Error(err)
				}
				_ = response.Body.Close()
				actualTrimmedBody := strings.TrimSpace(string(body))
				if actualTrimmedBody != tc.expectedBody {
					t.Errorf("expected body '%s', got '%s'", tc.expectedBody, actualTrimmedBody)
				}
			}
		})
	}
}

type MockProvider struct {
	t        *testing.T
	testCase testCase
}

// Records MockProvider implementation to be removed when real providers are added
func (d *MockProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	return d.testCase.returnRecords, d.testCase.hasError
}

// ApplyChanges MockProvider implementation to be removed when real providers are added
func (d *MockProvider) ApplyChanges(_ context.Context, changes *plan.Changes) error {
	if d.testCase.hasError != nil {
		return d.testCase.hasError
	}
	if !reflect.DeepEqual(changes, d.testCase.expectedChanges) {
		d.t.Errorf("expected changes '%v', got '%v'", d.testCase.expectedChanges, changes)
	}
	return nil
}

func (d *MockProvider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	if !reflect.DeepEqual(endpoints, d.testCase.expectedEndpointsToAdjust) {
		d.t.Errorf("expected endpoints to adjust '%v', got '%v'", d.testCase.expectedEndpointsToAdjust, endpoints)
	}
	return d.testCase.returnAdjustedEndpoints, nil
}

func (d *MockProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	return d.testCase.returnDomainFilter
}
