package s3

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// mockAPIError implements smithy.APIError for testing.
type mockAPIError struct {
	code    string
	message string
}

func (e *mockAPIError) Error() string                 { return e.message }
func (e *mockAPIError) ErrorCode() string             { return e.code }
func (e *mockAPIError) ErrorMessage() string          { return e.message }
func (e *mockAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

func TestIsBucketAlreadyOwnedByYou(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic error",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "BucketAlreadyOwnedByYou typed error",
			err:  &types.BucketAlreadyOwnedByYou{},
			want: true,
		},
		{
			name: "BucketAlreadyExists typed error",
			err:  &types.BucketAlreadyExists{},
			want: true,
		},
		{
			name: "BucketAlreadyOwnedByYou API error code",
			err:  &mockAPIError{code: "BucketAlreadyOwnedByYou", message: "bucket exists"},
			want: true,
		},
		{
			name: "BucketAlreadyExists API error code",
			err:  &mockAPIError{code: "BucketAlreadyExists", message: "bucket exists"},
			want: true,
		},
		{
			name: "other API error code",
			err:  &mockAPIError{code: "AccessDenied", message: "access denied"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBucketAlreadyOwnedByYou(tt.err)
			if got != tt.want {
				t.Errorf("isBucketAlreadyOwnedByYou() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic error",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "NoSuchBucket typed error",
			err:  &types.NoSuchBucket{},
			want: true,
		},
		{
			name: "NotFound typed error",
			err:  &types.NotFound{},
			want: true,
		},
		{
			name: "NotFound API error code",
			err:  &mockAPIError{code: "NotFound", message: "not found"},
			want: true,
		},
		{
			name: "NoSuchBucket API error code",
			err:  &mockAPIError{code: "NoSuchBucket", message: "bucket not found"},
			want: true,
		},
		{
			name: "404 API error code",
			err:  &mockAPIError{code: "404", message: "not found"},
			want: true,
		},
		{
			name: "other API error code",
			err:  &mockAPIError{code: "AccessDenied", message: "access denied"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotFoundError(tt.err)
			if got != tt.want {
				t.Errorf("isNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}
