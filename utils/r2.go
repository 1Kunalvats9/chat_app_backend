package utils

import (
	"chat-backend/config"
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)
var R2Client *s3.Client
var R2BucketName string 
var R2PublicURL string 

	
func InitR2(cfg *config.Config) error {
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID),
		}, nil
	})

	awsCfg, err := awscfg.LoadDefaultConfig(context.TODO(),
		awscfg.WithEndpointResolverWithOptions(r2Resolver),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.R2AccessKeyID,
			cfg.R2SecretAccessKey,
			"",
		)),
		awscfg.WithRegion("auto"),
	)

	if err != nil {
		return fmt.Errorf("failed to initialize R2: %w", err)
	}

	R2Client = s3.NewFromConfig(awsCfg)
	R2BucketName = cfg.R2BucketName
	R2PublicURL = cfg.R2PublicURL

	return nil
}


type UploadResult struct {
	URL      string
	FileName string
	FileSize int64
	MimeType string
}

func UploadFile(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error) {
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()

	ext := filepath.Ext(file.Filename)
	fileName := fmt.Sprintf("%s/%s%s", folder, uuid.New().String(), ext)
	mimeType := file.Header.Get("Content-Type")

	_, err = R2Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(R2BucketName),
		Key:           aws.String(fileName),
		Body:          src,
		ContentType:   aws.String(mimeType),
		ContentLength: aws.Int64(file.Size),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	url := fmt.Sprintf("%s/%s", R2PublicURL, fileName)

	return &UploadResult{
		URL:      url,
		FileName: file.Filename,
		FileSize: file.Size,
		MimeType: mimeType,
	}, nil
}

func DeleteFile(ctx context.Context, fileURL string) error {
	key := strings.TrimPrefix(fileURL, R2PublicURL+"/")

	_, err := R2Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(R2BucketName),
		Key:    aws.String(key),
	})

	return err
}

func GeneratePresignedURL(ctx context.Context, fileName string, expiry time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(R2Client)

	req, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(R2BucketName),
		Key:    aws.String(fileName),
	}, s3.WithPresignExpires(expiry))

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return req.URL, nil
}