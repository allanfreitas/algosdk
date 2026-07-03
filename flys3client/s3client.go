package flys3client

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config stores config related to AWS S3.
type S3Config struct {
	AccessKeyID          string
	SecretAccessKey      string
	Region               string
	Bucket               string
	UsePathStyleEndpoint bool
	Endpoint             string
}

// Client wraps the AWS S3 client with the configured bucket name.
type Client struct {
	s3     *s3.Client
	bucket string
}

// New creates an S3 Client from application config.
func New(cfg S3Config) (*Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("s3client: load aws config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if cfg.UsePathStyleEndpoint {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	return &Client{
		s3:     s3.NewFromConfig(awsCfg, s3Opts...),
		bucket: cfg.Bucket,
	}, nil
}

// GetObject returns a streaming reader for an S3 object.
// Caller is responsible for closing the returned ReadCloser.
func (c *Client) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3client: get object %q: %w", key, err)
	}
	return out.Body, nil
}

// StreamCSV opens the S3 object at key and invokes callback for every data row.
// The header row (first line) is skipped automatically.
// Returns the total number of processed rows or the first error encountered.
func (c *Client) StreamCSV(ctx context.Context, key string, callback func(row []string) error) (int, error) {
	body, err := c.GetObject(ctx, key)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	r := csv.NewReader(body)
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	// Skip header row
	if _, err := r.Read(); err != nil {
		return 0, fmt.Errorf("s3client: read header: %w", err)
	}

	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("s3client: read row %d: %w", count+1, err)
		}
		if err := callback(record); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
