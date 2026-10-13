package storage

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/levmv/imgserv/config"
)

type captureS3Request func(*http.Request) (*http.Response, error)

func (f captureS3Request) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestS3EndpointAndSigning(t *testing.T) {
	credentials := writeTestS3Credentials(t)
	for _, test := range []struct {
		name     string
		bucket   string
		region   string
		endpoint string
		baseURL  string
	}{
		{
			name: "legacy Yandex", bucket: "test-bucket", region: "ru-central1",
			baseURL: "https://test-bucket.storage.yandexcloud.net",
		},
		{
			name: "legacy Yandex dotted bucket", bucket: "test.bucket", region: "ru-central1",
			baseURL: "https://storage.yandexcloud.net/test.bucket",
		},
		{
			name: "Selectel vHosted", bucket: "test-bucket", region: "ru-6",
			endpoint: "https://s3.ru-6.storage.selcloud.ru",
			baseURL:  "https://test-bucket.s3.ru-6.storage.selcloud.ru",
		},
		{
			name: "explicit endpoint overrides Yandex", bucket: "test-bucket", region: "ru-central1",
			endpoint: "https://storage.example.test",
			baseURL:  "https://test-bucket.storage.example.test",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := NewS3Storage(config.StorageConf{
				Bucket: test.bucket, Credentials: credentials,
				Region: test.region, Endpoint: test.endpoint,
			})
			if err != nil {
				t.Fatal(err)
			}
			transport := captureS3Request(func(r *http.Request) (*http.Response, error) {
				if got, want := r.URL.Scheme+"://"+r.URL.Host+r.URL.EscapedPath(), test.baseURL+"/nested/photo%20key.jpg"; got != want {
					t.Errorf("request URL = %q, want %q", got, want)
				}
				auth := r.Header.Get("Authorization")
				if !strings.Contains(auth, "Credential=test-key/") || !strings.Contains(auth, "/"+test.region+"/s3/aws4_request") {
					t.Errorf("request is not signed with the configured credentials and region: %q", auth)
				}
				return &http.Response{
					StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader("")), Request: r,
				}, nil
			})
			key := "nested/photo key.jpg"
			result, err := store.client.GetObject(t.Context(), &s3.GetObjectInput{
				Bucket: &store.Bucket, Key: &key,
			}, func(options *s3.Options) { options.HTTPClient = transport })
			if err != nil {
				t.Fatal(err)
			}
			result.Body.Close()
		})
	}
}

func writeTestS3Credentials(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(path, []byte("[default]\naws_access_key_id = test-key\naws_secret_access_key = test-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
