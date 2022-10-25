# Updated S3 for CertMagic Storage

This library allows you to use any S3-compatible provider as key/certificate storage backend for your [Certmagic](https://github.com/caddyserver/certmagic)-enabled HTTPS server. To protect your keys from unwanted attention, client-side encryption using [secretbox](https://pkg.go.dev/golang.org/x/crypto@v0.0.0-20200728195943-123391ffb6de/nacl/secretbox?tab=doc) is possible.

See example/ for an exemplary integration.

## Why have we made this fork?
Whilst using this plugin, Certmagic itself calls the Load and other functions quite a lot and there is not any level of caching on those functions for the library. We've chosen BadgerDB which is a proven database that has been able to handle millions of concurrent reads and writes on our systems. We've learned that the default S3 cache library simply cannot cut it and handle the amount of requests we receive. 

The aim of this fork is to improve performance and scalability when it comes to using the AWS S3 storage with Certmagic to store certificates.

## What is a S3-compatible service?

In the current state, any service must support the following:

- v4 Signatures
- HTTPS
- A few basic operations:
	- Bucket Exists
	- Get Object
	- Put Object
	- Remove Object
	- Stat Object
	- List Objects

Known good providers/software:

- Minio (with HTTPS enabled)
- Backblaze
- AWS

### For development
Our caching key format is as follows

- `<key>` - Just a regular S3 file
- `<key_ki> - The key info for a S3 file
