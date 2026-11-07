package integration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

type OSSConfig struct {
	Enabled         bool
	Mock            bool
	UseCName        bool
	Region          string
	Bucket          string
	Endpoint        string
	Prefix          string
	AccessKeyID     string
	AccessKeySecret string
	SecurityToken   string
}

var regionPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)
var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
var prefixPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$`)
var ErrObjectExists = errors.New("OSS object already exists")
var ErrObjectMissing = errors.New("OSS object missing")
var ErrObjectUnavailable = errors.New("OSS object unavailable")

func (c OSSConfig) Validate(production bool) error {
	if !c.Enabled {
		if c.Mock {
			return errors.New("invalid OA_OSS_MOCK")
		}
		return nil
	}
	if !regionPattern.MatchString(c.Region) || !bucketPattern.MatchString(c.Bucket) {
		return errors.New("invalid OA_OSS_REGION or OA_OSS_BUCKET")
	}
	if len(c.AccessKeyID) < 4 || len(c.AccessKeySecret) < 8 ||
		strings.ContainsAny(c.AccessKeyID+c.AccessKeySecret+c.SecurityToken, " \r\n\t") {
		return errors.New("invalid OSS credential configuration")
	}
	if c.Prefix != "" && (len(c.Prefix) > 128 || !prefixPattern.MatchString(c.Prefix)) {
		return errors.New("invalid OA_OSS_PREFIX")
	}
	if c.Endpoint != "" {
		url, valid := endpoint(c.Endpoint, false)
		if !valid || (c.Mock && !localHost(url.Hostname())) ||
			(!c.Mock && (url.Scheme != "https" || localHost(url.Hostname()) ||
				(!c.UseCName && !strings.HasSuffix(strings.ToLower(url.Hostname()), ".aliyuncs.com")))) {
			return errors.New("invalid OA_OSS_ENDPOINT")
		}
	}
	if (c.Mock && (production || c.Endpoint == "" || c.UseCName)) || (c.UseCName && c.Endpoint == "") {
		return errors.New("invalid OSS mock or CNAME configuration")
	}
	return nil
}

type OSS struct {
	client *oss.Client
	bucket string
	prefix string
	Mock   bool
}

func NewOSS(config OSSConfig) *OSS {
	if !config.Enabled {
		return nil
	}
	cfg := oss.LoadDefaultConfig().
		WithRegion(config.Region).
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.AccessKeySecret, config.SecurityToken)).
		WithRetryMaxAttempts(1).
		WithHttpClient(&http.Client{
			Timeout:       12 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}).
		WithUseCName(config.UseCName).WithUsePathStyle(config.Mock)
	if config.Endpoint != "" {
		cfg.WithEndpoint(strings.TrimRight(config.Endpoint, "/"))
	}
	prefix := config.Prefix
	if prefix == "" {
		prefix = "hr-oa"
	}
	return &OSS{client: oss.NewClient(cfg), bucket: config.Bucket, prefix: prefix + "/", Mock: config.Mock}
}

func (o *OSS) ValidateBucket(ctx context.Context) error {
	acl, err := o.client.GetBucketAcl(ctx, &oss.GetBucketAclRequest{Bucket: &o.bucket})
	if err != nil || acl.ACL == nil || *acl.ACL != "private" {
		return errors.New("OSS bucket ACL is not verified private")
	}
	block, err := o.client.GetBucketPublicAccessBlock(ctx, &oss.GetBucketPublicAccessBlockRequest{Bucket: &o.bucket})
	if err != nil || block.PublicAccessBlockConfiguration == nil || block.PublicAccessBlockConfiguration.BlockPublicAccess == nil || !*block.PublicAccessBlockConfiguration.BlockPublicAccess {
		return errors.New("OSS bucket public access block is not verified")
	}
	versioning, err := o.client.GetBucketVersioning(ctx, &oss.GetBucketVersioningRequest{Bucket: &o.bucket})
	if err != nil || (versioning.VersionStatus != nil && *versioning.VersionStatus != "") {
		return errors.New("OSS bucket must have versioning unset for no-overwrite writes")
	}
	return nil
}

func (o *OSS) Put(ctx context.Context, key, contentType string, data []byte) error {
	result, err := o.client.PutObject(ctx, &oss.PutObjectRequest{
		Bucket: &o.bucket, Key: oss.Ptr(o.prefix + key), Body: bytes.NewReader(data),
		ContentLength: oss.Ptr(int64(len(data))), ContentType: &contentType,
		CacheControl: oss.Ptr("no-store"), ContentDisposition: oss.Ptr("attachment"),
		Acl: oss.ObjectACLPrivate, ForbidOverwrite: oss.Ptr("true"), ServerSideEncryption: oss.Ptr("AES256"),
	})
	var remote *oss.ServiceError
	if errors.As(err, &remote) && remote.Code == "FileAlreadyExists" {
		return ErrObjectExists
	}
	if err != nil || result == nil {
		return ErrObjectUnavailable
	}
	return nil
}

func (o *OSS) Read(ctx context.Context, key string, size int64) ([]byte, error) {
	reply, err := o.client.GetObject(ctx, &oss.GetObjectRequest{Bucket: &o.bucket, Key: oss.Ptr(o.prefix + key)})
	var remote *oss.ServiceError
	if errors.As(err, &remote) && remote.Code == "NoSuchKey" {
		return nil, ErrObjectMissing
	}
	if err != nil || reply == nil || reply.Body == nil {
		return nil, ErrObjectUnavailable
	}
	defer reply.Body.Close()
	if reply.ContentLength != -1 && reply.ContentLength != size {
		return nil, ErrObjectUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(reply.Body, size+1))
	if err != nil || int64(len(data)) != size {
		return nil, ErrObjectUnavailable
	}
	return data, nil
}
