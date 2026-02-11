package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// testClient creates a Client backed by a test HTTP server.
// The handler receives real S3 XML-protocol requests.
func testClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)

	client := s3.New(s3.Options{
		Region:       "fsn1",
		BaseEndpoint: aws.String(server.URL),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider("test-key", "test-secret", ""),
		HTTPClient: &http.Client{
			Transport: &http.Transport{},
		},
	})

	return &Client{s3: client, region: "fsn1"}, server
}

// xmlResponse is a helper to write S3-style XML responses.
func xmlResponse(w http.ResponseWriter, statusCode int, body string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		endpoint  string
		region    string
		accessKey string
		secretKey string
		wantErr   bool
	}{
		{
			name:      "valid credentials",
			endpoint:  "https://fsn1.your-objectstorage.com",
			region:    "fsn1",
			accessKey: "test-access-key",
			secretKey: "test-secret-key",
			wantErr:   false,
		},
		{
			name:      "empty credentials still succeeds at client creation",
			endpoint:  "https://fsn1.your-objectstorage.com",
			region:    "fsn1",
			accessKey: "",
			secretKey: "",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, err := NewClient(tt.endpoint, tt.region, tt.accessKey, tt.secretKey)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if client == nil {
				t.Fatal("expected non-nil client")
			}
			if client.region != tt.region {
				t.Errorf("expected region %s, got %s", tt.region, client.region)
			}
		})
	}
}

func TestCreateBucket_Success(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			xmlResponse(w, 200, `<?xml version="1.0" encoding="UTF-8"?><CreateBucketResult/>`)
			return
		}
		xmlResponse(w, 404, "")
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.CreateBucket(context.Background(), "test-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateBucket_AlreadyOwnedByYou(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 409, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>BucketAlreadyOwnedByYou</Code>
  <Message>Your previous request to create the named bucket succeeded and you already own it.</Message>
  <BucketName>test-bucket</BucketName>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.CreateBucket(context.Background(), "test-bucket")
	if err != nil {
		t.Fatalf("expected nil error for already owned bucket, got: %v", err)
	}
}

func TestCreateBucket_Error(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 403, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>AccessDenied</Code>
  <Message>Access Denied</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.CreateBucket(context.Background(), "test-bucket")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "failed to create bucket test-bucket") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBucketExists_True(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	exists, err := client.BucketExists(context.Background(), "test-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected bucket to exist")
	}
}

func TestBucketExists_False(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 404, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>NotFound</Code>
  <Message>Not Found</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	exists, err := client.BucketExists(context.Background(), "nonexistent-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected bucket to not exist")
	}
}

func TestBucketExists_OtherError(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 403, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>AccessDenied</Code>
  <Message>Access Denied</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	_, err := client.BucketExists(context.Background(), "test-bucket")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "failed to check bucket test-bucket") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestPutObject_Success(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	var mu sync.Mutex

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			mu.Lock()
			body, _ := io.ReadAll(r.Body)
			capturedBody = body
			mu.Unlock()
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	data := []byte("hello world")
	err := client.PutObject(context.Background(), "test-bucket", "test-key", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !bytes.Equal(capturedBody, data) {
		t.Errorf("expected body %q, got %q", data, capturedBody)
	}
}

func TestPutObject_Error(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 500, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>InternalError</Code>
  <Message>Internal Error</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.PutObject(context.Background(), "test-bucket", "test-key", []byte("data"))
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "failed to put object test-key in bucket test-bucket") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetObject_Success(t *testing.T) {
	t.Parallel()

	expectedData := []byte("object content here")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(expectedData)))
			w.WriteHeader(200)
			_, _ = w.Write(expectedData)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	data, err := client.GetObject(context.Background(), "test-bucket", "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, expectedData) {
		t.Errorf("expected %q, got %q", expectedData, data)
	}
}

func TestGetObject_Error(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 404, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>NoSuchKey</Code>
  <Message>The specified key does not exist.</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	_, err := client.GetObject(context.Background(), "test-bucket", "missing-key")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "failed to get object missing-key from bucket test-bucket") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDeleteObject_Success(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.DeleteObject(context.Background(), "test-bucket", "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteObject_Error(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 500, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>InternalError</Code>
  <Message>Internal Error</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.DeleteObject(context.Background(), "test-bucket", "test-key")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "failed to delete object test-key from bucket test-bucket") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDeleteBucket_Success(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.DeleteBucket(context.Background(), "test-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteBucket_Error(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 409, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>BucketNotEmpty</Code>
  <Message>The bucket you tried to delete is not empty</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.DeleteBucket(context.Background(), "test-bucket")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "failed to delete bucket test-bucket") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestListObjects_Success(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			xmlResponse(w, 200, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>test-bucket</Name>
  <Prefix></Prefix>
  <KeyCount>3</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents>
    <Key>file1.txt</Key>
    <Size>100</Size>
  </Contents>
  <Contents>
    <Key>file2.txt</Key>
    <Size>200</Size>
  </Contents>
  <Contents>
    <Key>dir/file3.txt</Key>
    <Size>300</Size>
  </Contents>
</ListBucketResult>`)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	keys, err := client.ListObjects(context.Background(), "test-bucket", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	expectedKeys := map[string]bool{"file1.txt": true, "file2.txt": true, "dir/file3.txt": true}
	for _, key := range keys {
		if !expectedKeys[key] {
			t.Errorf("unexpected key: %s", key)
		}
	}
}

func TestListObjects_WithPrefix(t *testing.T) {
	t.Parallel()

	var capturedPrefix string
	var mu sync.Mutex

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			mu.Lock()
			capturedPrefix = r.URL.Query().Get("prefix")
			mu.Unlock()
			xmlResponse(w, 200, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>test-bucket</Name>
  <Prefix>dir/</Prefix>
  <KeyCount>1</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents>
    <Key>dir/file3.txt</Key>
    <Size>300</Size>
  </Contents>
</ListBucketResult>`)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	keys, err := client.ListObjects(context.Background(), "test-bucket", "dir/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0] != "dir/file3.txt" {
		t.Errorf("expected key dir/file3.txt, got %s", keys[0])
	}

	mu.Lock()
	defer mu.Unlock()
	if capturedPrefix != "dir/" {
		t.Errorf("expected prefix dir/, got %s", capturedPrefix)
	}
}

func TestListObjects_Empty(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 200, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>test-bucket</Name>
  <Prefix></Prefix>
  <KeyCount>0</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
</ListBucketResult>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	keys, err := client.ListObjects(context.Background(), "test-bucket", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

func TestListObjects_Error(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 404, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>NoSuchBucket</Code>
  <Message>The specified bucket does not exist</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	_, err := client.ListObjects(context.Background(), "nonexistent-bucket", "")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "failed to list objects in bucket nonexistent-bucket") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestWriteMetadata_Success(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	var capturedKey string
	var mu sync.Mutex

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			mu.Lock()
			capturedKey = strings.TrimPrefix(r.URL.Path, "/test-bucket/")
			body, _ := io.ReadAll(r.Body)
			capturedBody = body
			mu.Unlock()
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.WriteMetadata(context.Background(), "test-bucket", "my-cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedKey != MetadataFileName {
		t.Errorf("expected key %s, got %s", MetadataFileName, capturedKey)
	}

	var metadata BucketMetadata
	if err := json.Unmarshal(capturedBody, &metadata); err != nil {
		t.Fatalf("failed to unmarshal captured body: %v", err)
	}
	if metadata.ClusterName != "my-cluster" {
		t.Errorf("expected cluster name my-cluster, got %s", metadata.ClusterName)
	}
	if metadata.ManagedBy != "k8zner" {
		t.Errorf("expected managedBy k8zner, got %s", metadata.ManagedBy)
	}
	if metadata.CreatedAt == "" {
		t.Error("expected non-empty createdAt")
	}
}

func TestGetMetadata_Success(t *testing.T) {
	t.Parallel()

	metadata := BucketMetadata{
		ClusterName: "test-cluster",
		ManagedBy:   "k8zner",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}
	metaBytes, _ := json.Marshal(metadata)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(metaBytes)))
			w.WriteHeader(200)
			_, _ = w.Write(metaBytes)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	result, err := client.GetMetadata(context.Background(), "test-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil metadata")
	}
	if result.ClusterName != "test-cluster" {
		t.Errorf("expected cluster name test-cluster, got %s", result.ClusterName)
	}
	if result.ManagedBy != "k8zner" {
		t.Errorf("expected managedBy k8zner, got %s", result.ManagedBy)
	}
}

func TestGetMetadata_NotFound_NoSuchBucket(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 404, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>NoSuchBucket</Code>
  <Message>The specified bucket does not exist.</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	result, err := client.GetMetadata(context.Background(), "test-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil metadata for not-found, got %+v", result)
	}
}

func TestGetMetadata_NotFound_NotFoundCode(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 404, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>NotFound</Code>
  <Message>Not Found</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	result, err := client.GetMetadata(context.Background(), "test-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil metadata for not-found, got %+v", result)
	}
}

func TestGetMetadata_InvalidJSON(t *testing.T) {
	t.Parallel()

	invalidJSON := []byte("not valid json{{{")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(invalidJSON)))
			w.WriteHeader(200)
			_, _ = w.Write(invalidJSON)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	_, err := client.GetMetadata(context.Background(), "test-bucket")
	if err == nil {
		t.Fatal("expected error for invalid JSON metadata")
	}
	if !strings.Contains(err.Error(), "failed to unmarshal metadata") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetMetadata_OtherError(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, 500, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>InternalError</Code>
  <Message>Internal Error</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	_, err := client.GetMetadata(context.Background(), "test-bucket")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

func TestCreateBucketWithMetadata_Success(t *testing.T) {
	t.Parallel()

	requestCount := 0
	var mu sync.Mutex

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		if r.Method == "PUT" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.CreateBucketWithMetadata(context.Background(), "test-bucket", "my-cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateBucketWithMetadata_CreateBucketError(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First request is CreateBucket - return error
		xmlResponse(w, 403, `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>AccessDenied</Code>
  <Message>Access Denied</Message>
</Error>`)
	})

	client, server := testClient(t, handler)
	defer server.Close()

	err := client.CreateBucketWithMetadata(context.Background(), "test-bucket", "my-cluster")
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "failed to create bucket") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBucketMetadata_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := BucketMetadata{
		ClusterName: "my-cluster",
		ManagedBy:   "k8zner",
		CreatedAt:   "2026-02-10T12:00:00Z",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded BucketMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ClusterName != original.ClusterName {
		t.Errorf("ClusterName: expected %s, got %s", original.ClusterName, decoded.ClusterName)
	}
	if decoded.ManagedBy != original.ManagedBy {
		t.Errorf("ManagedBy: expected %s, got %s", original.ManagedBy, decoded.ManagedBy)
	}
	if decoded.CreatedAt != original.CreatedAt {
		t.Errorf("CreatedAt: expected %s, got %s", original.CreatedAt, decoded.CreatedAt)
	}
}

func TestMetadataFileName_Constant(t *testing.T) {
	t.Parallel()
	if MetadataFileName != "k8zner_metadata.json" {
		t.Errorf("expected MetadataFileName to be k8zner_metadata.json, got %s", MetadataFileName)
	}
}

// Test isBucketAlreadyOwnedByYou with wrapped errors.
func TestIsBucketAlreadyOwnedByYou_WrappedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "wrapped BucketAlreadyOwnedByYou",
			err:  fmt.Errorf("outer: %w", &s3types.BucketAlreadyOwnedByYou{}),
			want: true,
		},
		{
			name: "wrapped BucketAlreadyExists",
			err:  fmt.Errorf("outer: %w", &s3types.BucketAlreadyExists{}),
			want: true,
		},
		{
			name: "wrapped generic error",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("inner error")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isBucketAlreadyOwnedByYou(tt.err)
			if got != tt.want {
				t.Errorf("isBucketAlreadyOwnedByYou() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test isNotFoundError with wrapped errors.
func TestIsNotFoundError_WrappedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "wrapped NoSuchBucket",
			err:  fmt.Errorf("outer: %w", &s3types.NoSuchBucket{}),
			want: true,
		},
		{
			name: "wrapped NotFound",
			err:  fmt.Errorf("outer: %w", &s3types.NotFound{}),
			want: true,
		},
		{
			name: "wrapped generic error",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("inner error")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isNotFoundError(tt.err)
			if got != tt.want {
				t.Errorf("isNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}
