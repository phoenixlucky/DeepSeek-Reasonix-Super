package provider

// The identities of why a video was not sent, said to the model beside the file's
// name so it does not invent a reason. Every adapter that sends video uses these.
const (
	VideoFileMissing  = "video.file_missing"
	VideoUnavailable  = "video.file_unavailable"
	VideoFileChanged  = "video.file_changed"
	VideoTooLarge     = "video.native_too_large"
	VideoUploadFailed = "video.upload_failed"
	VideoReadFailed   = "video.read_failed"
)
