package netio

// Adjust buffer to optimal size based on file size to be received.
//
// Parameters:
// - fileSize:  The size of the file to be received
//
// Returns:
// - An optimal integer buffer size
//
func GetOptimalBufferSize(fileSize int) int {
	switch {
	// If the file is less than or equal to 1 MB
	case fileSize <= 1 * 1024 * 1024:
		// 512 byte buffer
		return 512
	// If the file is less than or equal to 100 MB
	case fileSize <= 100 * 1024 * 1024:
		// 8 KB buffer
		return 8 * 1024
	// If the file is greater than 100 MB
	default:
		// 1 MB buffer
		return 1024 * 1024
	}
}
