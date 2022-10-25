package badgers3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/caddyserver/certmagic"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"time"
)

type S3Opts struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string

	ObjPrefix string

	// EncryptionKey is optional. If you do not wish to encrypt your certficates and key inside the S3 bucket, leave it empty.
	EncryptionKey []byte
}

type S3Storage struct {
	prefix   string
	bucket   string
	s3client *minio.Client

	iowrap IO
}

func NewS3Storage(opts S3Opts) (*S3Storage, error) {
	gs3 := &S3Storage{
		prefix: opts.ObjPrefix,
		bucket: opts.Bucket,
	}

	if opts.EncryptionKey == nil || len(opts.EncryptionKey) == 0 {
		log.Println("Clear text certificate storage active")
		gs3.iowrap = &CleartextIO{}
	} else if len(opts.EncryptionKey) != 32 {
		return nil, errors.New("encryption key must have exactly 32 bytes")
	} else {
		log.Println("Encrypted certificate storage active")
		sb := &SecretBoxIO{}
		copy(sb.SecretKey[:], opts.EncryptionKey)
		gs3.iowrap = sb
	}

	var err error
	gs3.s3client, err = minio.New(opts.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(opts.AccessKeyID, opts.SecretAccessKey, ""),
		Secure: true,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, err := gs3.s3client.BucketExists(ctx, opts.Bucket)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("S3 bucket %s does not exist", opts.Bucket)
	}
	return gs3, nil
}

var (
	LockExpiration   = 2 * time.Minute
	LockPollInterval = 1 * time.Second
	LockTimeout      = 15 * time.Second
)

func (gs *S3Storage) Lock(ctx context.Context, key string) error {
	// There is no need to lock any file if it is cached so we return if it is cached
	if isCacheEntryExistent([]byte(key)) {
		return nil
	}

	var startedAt = time.Now()

	for {
		obj, err := gs.s3client.GetObject(ctx, gs.bucket, gs.objLockName(key), minio.GetObjectOptions{})
		if err == nil {
			return gs.putLockFile(key)
		}
		buf, err := ioutil.ReadAll(obj)
		if err != nil {
			// Retry
			continue
		}
		lt, err := time.Parse(time.RFC3339, string(buf))
		if err != nil {
			// Lock file does not make sense, overwrite.
			return gs.putLockFile(key)
		}
		if lt.Add(LockTimeout).Before(time.Now()) {
			// Existing lock file expired, overwrite.
			return gs.putLockFile(key)
		}

		if startedAt.Add(LockTimeout).Before(time.Now()) {
			return errors.New("acquiring lock failed")
		}
		time.Sleep(LockPollInterval)
	}
	return errors.New("locking failed")
}

func (gs *S3Storage) putLockFile(key string) error {
	// Object does not exist, we're creating a lock file.
	r := bytes.NewReader([]byte(time.Now().Format(time.RFC3339)))
	_, err := gs.s3client.PutObject(context.Background(), gs.bucket, gs.objLockName(key), r, int64(r.Len()), minio.PutObjectOptions{})
	return err
}

func (gs *S3Storage) Unlock(ctx context.Context, key string) error {
	// There is no need to unlock any file if it is cached so we return if it is cached
	if isCacheEntryExistent([]byte(key)) {
		return nil
	}

	return gs.s3client.RemoveObject(ctx, gs.bucket, gs.objLockName(key), minio.RemoveObjectOptions{})
}

func (gs *S3Storage) Store(ctx context.Context, key string, value []byte) error {
	r := gs.iowrap.ByteReader(value)
	_, err := gs.s3client.PutObject(ctx,
		gs.bucket,
		gs.objName(key),
		r,
		int64(r.Len()),
		minio.PutObjectOptions{},
	)
	return err
}

func (gs *S3Storage) Load(ctx context.Context, key string) ([]byte, error) {
	// We try to get the cached file from our storage here
	if isCacheEntryExistent([]byte(key)) {
		// Get the key info
		rawKi := getCacheEntry([]byte(key))
		if rawKi != nil {
			// We have the cached file, return it as a byte array
			return []byte(*rawKi), nil
		}
	}
	g
	r, err := gs.s3client.GetObject(ctx, gs.bucket, gs.objName(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, fs.ErrNotExist
	}
	defer r.Close()
	buf, err := io.ReadAll(gs.iowrap.WrapReader(r))
	if err != nil {
		return nil, fs.ErrNotExist
	}

	// We have gotten a file from S3, let's cache it, no need to do any marshalling here!
	setCacheEntry([]byte(key), buf, time.Hour*1)

	return buf, nil
}

func (gs *S3Storage) Delete(ctx context.Context, key string) error {
	return gs.s3client.RemoveObject(ctx, gs.bucket, gs.objName(key), minio.RemoveObjectOptions{})
}

func (gs *S3Storage) Exists(ctx context.Context, key string) bool {
	_, err := gs.s3client.StatObject(ctx, gs.bucket, gs.objName(key), minio.StatObjectOptions{})
	return err == nil
}

func (gs *S3Storage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	var keys []string
	for obj := range gs.s3client.ListObjects(ctx, gs.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: recursive,
	}) {
		keys = append(keys, obj.Key)
	}
	return keys, nil
}

func (gs *S3Storage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	var ki certmagic.KeyInfo

	// First we check if we've already cached the stat data for the file
	if isCacheEntryExistent([]byte(key + "_ki")) {
		// Get the key info
		rawKi := getCacheEntry([]byte(key + "_ki"))
		if rawKi != nil {
			// Ensure that we only continue the cache fetch process if the key exists

			// Deserialize
			err := json.Unmarshal([]byte(*rawKi), &ki)
			if err == nil {
				// Only return if we had no errors with deserialization and actually got the value
				return ki, nil
			}
		}
	}

	// This is the normal flow and will contact S3 for the data and then cache it afterwards
	oi, err := gs.s3client.StatObject(ctx, gs.bucket, gs.objName(key), minio.StatObjectOptions{})
	if err != nil {
		return ki, err
	}
	ki.Key = key
	ki.Size = oi.Size
	ki.Modified = oi.LastModified
	ki.IsTerminal = true

	// Store the info in the cache storage so we don't have to contact S3 again for a while
	jsonKi, err := json.Marshal(ki)
	if err == nil {
		// Only set when we know the JSON data is valid
		setCacheEntry([]byte(key+"_ki"), jsonKi, time.Hour*1)
	}

	// Return
	return ki, nil
}

func (gs *S3Storage) objName(key string) string {
	return gs.prefix + "/" + key
}

func (gs *S3Storage) objLockName(key string) string {
	return gs.objName(key) + ".lock"
}
