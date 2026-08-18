package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/smithy-go"
)

type fakeAPIError struct{ code string }

func (e *fakeAPIError) Error() string                 { return e.code }
func (e *fakeAPIError) ErrorCode() string             { return e.code }
func (e *fakeAPIError) ErrorMessage() string          { return e.code }
func (e *fakeAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func TestIsAPIErrorCode(t *testing.T) {
	if isAPIErrorCode(nil, "NoSuchBucketPolicy") {
		t.Error("nil error should never match")
	}
	if isAPIErrorCode(errors.New("boom"), "NoSuchBucketPolicy") {
		t.Error("plain error should never match")
	}
	if !isAPIErrorCode(&fakeAPIError{code: "NoSuchBucketPolicy"}, "NoSuchBucketPolicy") {
		t.Error("matching API error code should match")
	}
	if isAPIErrorCode(&fakeAPIError{code: "AccessDenied"}, "NoSuchBucketPolicy") {
		t.Error("different API error code should not match")
	}
	wrapped := errors.New("wrapped: " + (&fakeAPIError{code: "NoSuchPublicAccessBlockConfiguration"}).Error())
	if isAPIErrorCode(wrapped, "NoSuchPublicAccessBlockConfiguration") {
		t.Error("errors.As should not match a string-wrapped error, only a real chain")
	}
}

func TestBucket(t *testing.T) {
	bucket := Bucket{
		Name:         "test-bucket",
		CreationDate: "2024-01-01 12:00:00",
		Region:       "eu-west-1",
	}

	if bucket.Name != "test-bucket" {
		t.Errorf("Expected bucket name 'test-bucket', got '%s'", bucket.Name)
	}

	if bucket.Region != "eu-west-1" {
		t.Errorf("Expected region 'eu-west-1', got '%s'", bucket.Region)
	}
}

func TestBucketEmptyRegion(t *testing.T) {
	bucket := Bucket{
		Name:   "test-bucket",
		Region: "",
	}

	if bucket.Region != "" {
		t.Errorf("Expected empty region, got '%s'", bucket.Region)
	}
}

func TestBucketRegionFromLocationConstraint(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		expected   string
	}{
		{"empty means us-east-1", "", "us-east-1"},
		{"us-east-1 passthrough", "us-east-1", "us-east-1"},
		{"eu-west-1", "eu-west-1", "eu-west-1"},
		{"arbitrary region", "ap-southeast-4", "ap-southeast-4"},
	}

	for _, tt := range tests {
		if got := bucketRegionFromLocationConstraint(tt.constraint); got != tt.expected {
			t.Errorf("%s: bucketRegionFromLocationConstraint(%q) = %q, expected %q",
				tt.name, tt.constraint, got, tt.expected)
		}
	}
}

func TestS3Object(t *testing.T) {
	file := S3Object{
		Key:          "folder/file.txt",
		Size:         1024,
		LastModified: "2024-01-01 12:00:00",
		StorageClass: "STANDARD",
		IsFolder:     false,
	}

	if file.Key != "folder/file.txt" {
		t.Errorf("Expected key 'folder/file.txt', got '%s'", file.Key)
	}

	if file.Size != 1024 {
		t.Errorf("Expected size 1024, got %d", file.Size)
	}

	if file.IsFolder {
		t.Errorf("Expected IsFolder to be false, got true")
	}

	if file.StorageClass != "STANDARD" {
		t.Errorf("Expected storage class 'STANDARD', got '%s'", file.StorageClass)
	}
}

func TestS3ObjectFolder(t *testing.T) {
	folder := S3Object{
		Key:      "folder/",
		IsFolder: true,
	}

	if folder.Key != "folder/" {
		t.Errorf("Expected key 'folder/', got '%s'", folder.Key)
	}

	if !folder.IsFolder {
		t.Errorf("Expected IsFolder to be true, got false")
	}

	if folder.Size != 0 {
		t.Errorf("Expected size 0 for folder, got %d", folder.Size)
	}
}

func TestS3ListResult(t *testing.T) {
	token := "test-token"
	result := S3ListResult{
		Objects: []S3Object{
			{Key: "file1.txt", Size: 100, IsFolder: false},
			{Key: "folder/", IsFolder: true},
		},
		NextContinuationToken: &token,
		IsTruncated:           true,
	}

	if len(result.Objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(result.Objects))
	}

	if !result.IsTruncated {
		t.Errorf("Expected IsTruncated to be true, got false")
	}

	if result.NextContinuationToken == nil {
		t.Errorf("Expected NextContinuationToken to be set")
	} else if *result.NextContinuationToken != "test-token" {
		t.Errorf("Expected token 'test-token', got '%s'", *result.NextContinuationToken)
	}
}

func TestGetBool(t *testing.T) {
	if getBool(nil) != false {
		t.Errorf("Expected false for nil pointer, got true")
	}

	trueVal := true
	if getBool(&trueVal) != true {
		t.Errorf("Expected true, got false")
	}

	falseVal := false
	if getBool(&falseVal) != false {
		t.Errorf("Expected false, got true")
	}
}

func TestGetInt64(t *testing.T) {
	if getInt64(nil) != 0 {
		t.Errorf("Expected 0 for nil pointer, got %d", getInt64(nil))
	}

	val := int64(1024)
	if getInt64(&val) != 1024 {
		t.Errorf("Expected 1024, got %d", getInt64(&val))
	}
}

func TestGetInt32(t *testing.T) {
	ptr := getInt32(100)
	if ptr == nil {
		t.Errorf("Expected non-nil pointer")
	} else if *ptr != 100 {
		t.Errorf("Expected 100, got %d", *ptr)
	}
}

func TestS3ObjectVersion(t *testing.T) {
	version := S3ObjectVersion{
		Key:          "file.txt",
		VersionId:    "v1.0",
		IsLatest:     true,
		Size:         2048,
		LastModified: "2024-01-01 12:00:00",
		StorageClass: "STANDARD",
	}

	if version.Key != "file.txt" {
		t.Errorf("Expected key 'file.txt', got '%s'", version.Key)
	}

	if version.VersionId != "v1.0" {
		t.Errorf("Expected version ID 'v1.0', got '%s'", version.VersionId)
	}

	if !version.IsLatest {
		t.Errorf("Expected IsLatest to be true, got false")
	}

	if version.Size != 2048 {
		t.Errorf("Expected size 2048, got %d", version.Size)
	}
}

func TestTransferProgressListener(t *testing.T) {
	var lastBytes int64
	var lastTotal int64

	listener := transferProgressListener{
		callback: func(transferred, total int64) {
			lastBytes = transferred
			lastTotal = total
		},
	}

	listener.OnObjectBytesTransferred(context.Background(), &transfermanager.ObjectBytesTransferredEvent{BytesTransferred: 100, TotalBytes: 1000})
	if lastBytes != 100 {
		t.Errorf("Expected 100 bytes transferred, got %d", lastBytes)
	}
	if lastTotal != 1000 {
		t.Errorf("Expected 1000 total bytes, got %d", lastTotal)
	}

	listener.OnObjectBytesTransferred(context.Background(), &transfermanager.ObjectBytesTransferredEvent{BytesTransferred: 500, TotalBytes: 1000})
	if lastBytes != 500 {
		t.Errorf("Expected 500 bytes transferred, got %d", lastBytes)
	}

	listener.OnObjectBytesTransferred(context.Background(), &transfermanager.ObjectBytesTransferredEvent{BytesTransferred: 1000, TotalBytes: 1000})
	if lastBytes != 1000 {
		t.Errorf("Expected 1000 bytes transferred, got %d", lastBytes)
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		str      string
		substr   string
		expected bool
	}{
		{"HelloWorld", "hello", true},
		{"HelloWorld", "WORLD", true},
		{"HelloWorld", "test", false},
		{"test-file.txt", "FILE", true},
		{"", "test", false},
		{"test", "", true},
	}

	for _, tt := range tests {
		result := containsIgnoreCase(tt.str, tt.substr)
		if result != tt.expected {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, expected %v",
				tt.str, tt.substr, result, tt.expected)
		}
	}
}

func TestToLower(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HELLO", "hello"},
		{"HeLLo", "hello"},
		{"hello", "hello"},
		{"TEST123", "test123"},
		{"", ""},
	}

	for _, tt := range tests {
		result := toLower(tt.input)
		if result != tt.expected {
			t.Errorf("toLower(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		str      string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "test", false},
		{"test", "test", true},
		{"test", "", true},
		{"", "test", false},
	}

	for _, tt := range tests {
		result := contains(tt.str, tt.substr)
		if result != tt.expected {
			t.Errorf("contains(%q, %q) = %v, expected %v",
				tt.str, tt.substr, result, tt.expected)
		}
	}
}

func TestIndexOfSubstring(t *testing.T) {
	tests := []struct {
		str      string
		substr   string
		expected int
	}{
		{"hello world", "world", 6},
		{"hello world", "hello", 0},
		{"hello world", "test", -1},
		{"test", "test", 0},
		{"", "test", -1},
	}

	for _, tt := range tests {
		result := indexOfSubstring(tt.str, tt.substr)
		if result != tt.expected {
			t.Errorf("indexOfSubstring(%q, %q) = %d, expected %d",
				tt.str, tt.substr, result, tt.expected)
		}
	}
}

func TestS3VersionedCopySource(t *testing.T) {
	got := s3VersionedCopySource("my-bucket", "path/to/key.txt", "abc123")
	want := "my-bucket/path/to/key.txt?versionId=abc123"
	if got != want {
		t.Errorf("s3VersionedCopySource() = %q, want %q", got, want)
	}
}
