package file

import (
  "bufio"
  "bytes"
  "time"
  "io"
  "compress/gzip"
  "fmt"
)

type FileHeader struct {
  UploadDate time.Time
  ApiKeyID uint8
  MimeType, OriginalName string
}

type FileHandle struct {
  Header FileHeader
  source *gzip.Reader
  rawSource io.Reader
}

func (handle *FileHandle) Close() error {
  handle.source.Close()
  if cls, ok := handle.rawSource.(io.Closer); ok {
    return cls.Close()
  }
  return nil
}

func (handle *FileHandle) Read(bytes []byte) (n int, err error) {
  return handle.source.Read(bytes)
}

func (handle *FileHandle) WriteTo(w io.Writer) (n int64, err error) {
  defer handle.Close()
  return io.Copy(w, handle.source)
}

func WriteFile(dst io.Writer, info FileHeader, src io.Reader) (nWritten int, err error) {
  dest := bufio.NewWriter(dst)
  n, err := dest.Write([]byte{0xFA, 0xFA})
  if err != nil {
    return
  }
  nWritten += n

  var headerBuffer bytes.Buffer
  _, err = writeNumber(uint(info.UploadDate.UnixNano() / 10E8), 5, &headerBuffer)
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
  n64, err := writeNumber(uint(headLength), 2, dest)
  if err != nil {
    return
  }
  nWritten += int(n64)
  n64, err = headerBuffer.WriteTo(dest)
  if err != nil {
    return
  }
  nWritten += int(n64)
  err = dest.Flush()
  if err != nil {
    return
  }

  compressionWriter := gzip.NewWriter(dst)
  defer compressionWriter.Close()
  n64, err = io.Copy(compressionWriter, src)
  if err != nil {
    return
  }
  nWritten += int(n64)
  compressionWriter.Flush()
  return
}

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

func ReadFile(src io.Reader) (handle *FileHandle, err error) {
  handle = &FileHandle{rawSource: src}
  handle.Header, _, err = ReadFileHeader(src)
  if err == nil {
    var gz *gzip.Reader
    gz, err = gzip.NewReader(src)
    if err != nil {
      return
    }
    handle.source = gz
  }

  return
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
  singleByte := make([]byte, 1)
  for i := 0; i < byteCount; i++ {
    _, err = in.Read(singleByte)
    if err != nil {
      return
    }

    shift := uint(8 * (byteCount - i - 1))
    number = number | uint64(singleByte[0]) << shift
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
