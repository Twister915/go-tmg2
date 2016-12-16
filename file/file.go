package file

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// The header of a saved file
// This contains the essential metadata that's encoded in the first 50 (or so) bytes of a file
type FileHeader struct {
	// The exact time that the webserver received the upload
	UploadDate time.Time
	// The index of the API Key which was used to do this upload.
	// This is only really useful to the application which has the array of API keys
	ApiKeyID uint8
	// the mimetype and original name of the file for the browser.
	// These are simply provided values, and may not accurately reflect reality
	MimeType, OriginalName string
}

// A file that has been opened and can be read.
// Contains a read header, and privately contains a reader which will decompress
type FileHandle struct {
	// The loaded header
	Header FileHeader
	// This could be a decompressed reader, or it could be nil.
	// It's set when SetReadMode is called with DecompressReadMode
	decompressSource *gzip.Reader
	// The reader for the remainder of the file. This is the raw compressed data from the file.
	rawSource io.Reader
	// The source we're currently reading from. This points to either rawSource or decompressSource
	currentSource io.Reader
}

type ReadMode uint8

const (
	DecompressReadMode ReadMode = iota
	RawReadMode
)

// Set the read mode of the file to either RawReadMode or DecompressReadMode
// if you set it to DecompressReadMode, the file will be decompressed and the handle will read bytes that are from the original upload
// if you set it to RawReadMode, the handle will return the raw gziped bytes from the encoded file
func (handle *FileHandle) SetReadMode(mode ReadMode) {
	switch mode {
	case DecompressReadMode:
		if handle.decompressSource == nil {
			decompress, err := gzip.NewReader(handle.rawSource)
			if err != nil {
				panic(err)
			}
			handle.decompressSource = decompress
		}
		handle.currentSource = handle.decompressSource
	case RawReadMode:
		handle.currentSource = handle.rawSource
	default:
		panic("bad mode passed")
	}
}

// Closes the readers and performs other i/o cleanup
func (handle *FileHandle) Close() error {
	handle.decompressSource.Close()
	if cls, ok := handle.rawSource.(io.Closer); ok {
		return cls.Close()
	}
	handle.currentSource = nil
	return nil
}

// This will read bytes into the passed array, returning the number of bytes read and or an error
// this implements io.Reader
func (handle *FileHandle) Read(bytes []byte) (n int, err error) {
	return handle.currentSource.Read(bytes)
}

// This will write into the writer, all of the bytes from this file based on SetReadMode
// this implements io.WriterTo
func (handle *FileHandle) WriteTo(w io.Writer) (n int64, err error) {
	defer handle.Close()
	return io.Copy(w, handle)
}

// Writes to the provided writer, using metadata from the "FileHeader" using the file contained in
// the io.Reader src; so the call is something like this -> WriteFile(destination, info, sourceFile)
//
// Returns the number of bytes written, or an error
func WriteFile(dst io.Writer, info FileHeader, src io.Reader) (nWritten int, err error) {
	if dst == nil || src == nil {
		err = fmt.Errorf("you must provide a non nil dst and src")
		return
	}
	n, err := dst.Write([]byte{0xFA, 0xFA})
	if err != nil {
		return
	}
	nWritten += n

	var headerBuffer bytes.Buffer
	_, err = writeNumber(uint(info.UploadDate.UnixNano()/10E8), 5, &headerBuffer)
	if err != nil {
		return
	}
	err = headerBuffer.WriteByte(byte(info.ApiKeyID))
	if err != nil {
		return
	}
	_, err = writeLengthPrefixedString(info.MimeType, &headerBuffer)
	if err != nil {
		return
	}
	_, err = writeLengthPrefixedString(info.OriginalName, &headerBuffer)
	if err != nil {
		return
	}

	headLength := headerBuffer.Len()
	n64, err := writeNumber(uint(headLength), 2, dst)
	if err != nil {
		return
	}
	nWritten += int(n64)
	n64, err = headerBuffer.WriteTo(dst)
	if err != nil {
		return
	}
	nWritten += int(n64)
	if err != nil {
		return
	}

	compressionWriter := gzip.NewWriter(dst)
	defer compressionWriter.Close()
	defer compressionWriter.Flush()
	n64, err = io.Copy(compressionWriter, src)
	if err != nil {
		return
	}
	nWritten += int(n64)
	return
}

// Reads just the header from a source, and returns it in the struct, as well as the number of bytes read, and the error
func ReadFileHeader(src io.Reader) (header FileHeader, nRead int, err error) {
	twoBytes := make([]byte, 2)
	n, err := src.Read(twoBytes)
	if err != nil {
		return
	}
	nRead += n
	if twoBytes[0] != twoBytes[1] && twoBytes[0] != 0xFA {
		err = fmt.Errorf("file does not begin with header 0xFAFA")
		return
	}

	headerLength, err := readNumber(2, src)
	if err != nil {
		return
	}
	headerBytes := make([]byte, headerLength)

	n, err = src.Read(headerBytes)
	if err != nil {
		return
	}

	headerBuf := bytes.NewBuffer(headerBytes)
	secondsTime, err := readNumber(5, headerBuf)
	if err != nil {
		return
	}
	header.UploadDate = time.Unix(int64(secondsTime), 0)
	header.ApiKeyID, err = headerBuf.ReadByte()
	if err != nil {
		return
	}
	header.MimeType, _, err = readLengthPrefixedString(headerBuf)
	if err != nil {
		return
	}
	header.OriginalName, _, err = readLengthPrefixedString(headerBuf)
	if err != nil {
		return
	}
	return
}

// Reads from the reader, into a FileHandle, which can be used to read the entire file.
// May return an error
func ReadFile(src io.Reader) (handle *FileHandle, err error) {
	if src == nil {
		err = fmt.Errorf("you need to provide a non nil reader")
		return
	}
	handle = &FileHandle{rawSource: src}
	handle.Header, _, err = ReadFileHeader(src)
	if err != nil {
		return
	}
	handle.SetReadMode(RawReadMode)

	return
}

const charset = "abcdefghkmnoprstwxzABCDEFGHJKLMNPQRTWXY34689"

// Returns a random string that's good for human URLs.
func CreateRandomFileName(length int) string {
	if length <= 0 {
		return ""
	}

	return string([]rune(charset)[rand.Intn(len(charset))]) + CreateRandomFileName(length-1)
}

func writeNumber(number uint, byteCount int, out io.Writer) (n int64, err error) {
	var buffer bytes.Buffer
	for i := 0; i < byteCount; i++ {
		shift := uint(8 * (byteCount - i - 1))
		buffer.WriteByte(byte(number >> shift))
	}
	return buffer.WriteTo(out)
}

func readNumber(byteCount int, in io.Reader) (number uint64, err error) {
	bytez := make([]byte, byteCount)
	_, err = in.Read(bytez)
	if err != nil {
		return
	}
	for i := 0; i < byteCount; i++ {
		shift := uint(8 * (byteCount - i - 1))
		number = number | uint64(bytez[i])<<shift
	}
	return
}

func writeLengthPrefixedString(str string, out io.Writer) (n int64, err error) {
	var buffer bytes.Buffer
	writeNumber(uint(len(str)), 2, &buffer)
	buffer.WriteString(str)
	return buffer.WriteTo(out)
}

func readLengthPrefixedString(in io.Reader) (str string, n int64, err error) {
	length, err := readNumber(2, in)
	if err != nil {
		return
	}
	stringBytes := make([]byte, length)
	_, err = in.Read(stringBytes)
	if err != nil {
		return
	}
	str = string(stringBytes)
	n = 2 + int64(length)
	return
}
