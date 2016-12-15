package file

import (
  "testing"
  "os"
  "io"
  "io/ioutil"
  "bytes"
  "time"
  "fmt"
)

var testFileHeader = FileHeader{time.Unix(1481834107, 0), uint8(1), "image/png", "Screenshot at todo"}

func TestRead(t *testing.T) {
  target, err := os.Open("test_data/encoded")
  if err != nil {
    t.Error(err)
  }
  handle, err := ReadFile(target)

  header := handle.Header
  if header != testFileHeader {
    t.Error("header does not match")
  }
  checkAgainstReal(t, handle, "raw")
}

func TestWrite(t *testing.T) {
  var buffer bytes.Buffer
  source, err := os.Open("test_data/raw")
  _, err = WriteFile(&buffer, testFileHeader, source)
  if err != nil {
    t.Error(err)
  }
  checkAgainstReal(t, &buffer, "encoded")
}

func BenchmarkWrite(b *testing.B) {
  source, err := ioutil.ReadFile("test_data/raw")
  if err != nil {
    b.Error(err)
  }
  b.ResetTimer()
  for i := 0; i < b.N; i++ {
    var buffer bytes.Buffer
    WriteFile(&buffer, testFileHeader, bytes.NewReader(source))
  }
}

func BenchmarkRead(b *testing.B) {
  source, err := ioutil.ReadFile("test_data/encoded")
  if err != nil {
    b.Error(err)
  }

  b.ResetTimer()
  for i := 0; i < b.N; i++ {
    handle, err := ReadFile(bytes.NewReader(source))
    if err != nil {
      b.Error(err)
    }
    _, err = ioutil.ReadAll(handle)
    if err != nil {
      b.Error(err)
    }
  }
}

func checkAgainstReal(t *testing.T, handle io.Reader, real string) {
  realData, err := ioutil.ReadFile("test_data/" + real)
  if err != nil {
    t.Error(err)
  }
  ourData, err := ioutil.ReadAll(handle)
  if err != nil {
    t.Error(err)
  }

  if !bytes.Equal(realData, ourData) {
    var iS int
    for i := 0; i < localMin(len(realData), len(ourData)); i++ {
      if realData[i] == ourData[i] {
        continue
      }
      iS = i
      break
    }
    fmt.Printf("Inequality at %#x\n\t%v\n\t%v\n", iS, realData[iS:localMin(len(realData), iS + 20)], ourData[iS:localMin(len(ourData), iS + 20)])
    t.Error("equality check failed when comparing generated to test_data/" + real)
  }
}

func localMin(a, b int) int {
  if a < b {
    return a
  }
  return b
}
