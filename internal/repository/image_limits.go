package repository

// MaxImageSizeBytes caps a single uploaded image at 1.5 MB. Every presigned
// PUT is signed for the declared size, so S3 rejects a body of any other
// length; api/openapi.yaml's ImageUploadRequest.size_bytes carries the
// same bound for REST.
const MaxImageSizeBytes = 1536 * 1024

// MaxImageEdge is the longest side, in pixels, an uploaded image may have:
// what mykeebs-web scales an upload to fit. The moderation function rejects
// a larger image, since the presigned PUT can't check dimensions.
const MaxImageEdge = 2560

// MaxImagesPerItem caps the images on a keyboard or build. Switches, keycap
// kits and profiles have a single image slot and aren't affected.
const MaxImagesPerItem = 10
