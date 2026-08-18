package ui

import (
	"testing"

	"github.com/noelruault/lazyaws/apps/aws"
)

func TestS3ObjectsDrillDown(t *testing.T) {
	folder := s3ObjectsState{
		bucket:  "b",
		prefix:  "logs/",
		objects: []aws.S3Object{{Key: "logs/a.txt"}, {Key: "logs/2026/", IsFolder: true}},
	}

	got := s3ObjectsDrillDown(folder, 1)
	if got.bucket != "b" || got.prefix != "logs/2026/" || len(got.objects) != 0 {
		t.Errorf("s3ObjectsDrillDown(folder row) = %+v, want bucket=b prefix=logs/2026/ objects=nil", got)
	}

	if got := s3ObjectsDrillDown(folder, 0); got.prefix != folder.prefix || len(got.objects) != len(folder.objects) {
		t.Errorf("s3ObjectsDrillDown(file row) should no-op, got %+v, want %+v", got, folder)
	}

	if got := s3ObjectsDrillDown(folder, 5); got.prefix != folder.prefix {
		t.Errorf("s3ObjectsDrillDown(out-of-range cursor) should no-op, got %+v, want %+v", got, folder)
	}
}

func TestS3ObjectsDrillUp(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{"nested folder pops one segment", "logs/2026/", "logs/"},
		{"top-level folder pops to root", "logs/", ""},
		{"root is a no-op", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := s3ObjectsState{bucket: "b", prefix: tt.prefix}
			got := s3ObjectsDrillUp(start)
			if got.prefix != tt.want {
				t.Errorf("s3ObjectsDrillUp(%q) prefix = %q, want %q", tt.prefix, got.prefix, tt.want)
			}
			if tt.prefix == "" && got.bucket != start.bucket {
				t.Errorf("s3ObjectsDrillUp at root should return state unchanged, got %+v, want %+v", got, start)
			}
		})
	}
}

func TestS3ObjectRowCells(t *testing.T) {
	folder := s3ObjectRowCells(aws.S3Object{Key: "logs/2026/", IsFolder: true}, "logs/")
	if folder[1] != "2026/" {
		t.Errorf("folder name should strip the prefix, got %q", folder[1])
	}

	file := s3ObjectRowCells(aws.S3Object{Key: "logs/a.txt", Size: 1024, StorageClass: "STANDARD"}, "logs/")
	if file[1] != "a.txt" {
		t.Errorf("file name should strip the prefix, got %q", file[1])
	}
	if file[3] != "STANDARD" {
		t.Errorf("file storage class = %q, want STANDARD", file[3])
	}
}
