package globals

const GB = 1024 * 1024 * 1024
const MESSAGE_BUFFER_SIZE = 256

var COLON_DELIMITER = []byte(":")
var TRANSFER_REQUEST_MARKER = []byte("<TRANSFER_REQUEST>")
var START_TRANSFER_PREFIX = []byte("<START_TRANSFER:")
var START_TRANSFER_SUFFIX = []byte(">")
var END_TRANSFER_MARKER = []byte("<END_TRANSFER>")
var PROCESSING_COMPLETE = []byte("<PROCESSING_COMPLETE>")
var FILE_SIZE_TYPES = []string{"kb", "mb", "gb", "tb", "pb", "eb"}
