package rfslib

import (
    "flag"
    "fmt"
    "testing"
    "time"
)

var (
    localAddr = "127.0.0.1:8080"
    remoteMiner = "127.0.0.1:5050"
    rfs, _ = Initialize(localAddr,remoteMiner)
    localAddr2 = "127.0.0.1:9090"
    remoteMiner2 = "127.0.0.1:6060"
    rfs2, _= Initialize(localAddr2,remoteMiner2)
    rec1 Record
)

func TestMain(m *testing.M) {
    flag.Parse()
    m.Run()
}
func TestListFilesEmpty(t *testing.T) {
    fmt.Println("list files when empty")
    ls,err := rfs.ListFiles()
    if err != nil {
        t.Errorf("error occurred: %s", err)
    }
    fmt.Println(ls)
}
func TestCreateFile(t *testing.T) {
    fmt.Println("creating file")
    err := rfs.CreateFile("test")
    if err != nil {
        t.Errorf("error occurred: %s", err)
    } else { fmt.Println("creation succeeded") }
}
func TestCreateSecondFile(t *testing.T) {
    fmt.Println("creating second file")
    err := rfs.CreateFile("test2")
    if err != nil {
        t.Errorf("error occurred: %s", err)
    } else { fmt.Println("creation succeeded") }
    TestListFilesEmpty(t)
    fmt.Println("above should contain 2 files")
}
func TestAppendToFile(t *testing.T) {
    var byteArray [512]byte
    copy(byteArray[:], "test record")
    var testRecord Record = byteArray
    rec1 = testRecord
    fmt.Println("appending record to file test")

    recNum, err := rfs.AppendRec("test", &testRecord)
    if err != nil {
        t.Errorf("error occurred: %s", err)
    }
    if recNum != 0 {
        t.Errorf("error occurred: incorrect record number")
    }
}

func TestReadRecord(t *testing.T) {
    var rec = &Record{}
    err := rfs.ReadRec("test",0, rec)
    if *rec != rec1 {
        t.Errorf("error occurred: %s", err)
    }
}

func TestReadRecordNum(t *testing.T) {
    numRecs ,_ := rfs.TotalRecs("test")
    if int(numRecs) != 1 {
        t.Errorf("error occurred num records should be 1 but is:", numRecs)
    }
}

func TestCreateFileNameTooLong(t *testing.T) {
    fmt.Println("creating second file")
    err := rfs.CreateFile("test2ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
    if err != nil {
           fmt.Println("error returned: ", err)
    } else {
        t.Errorf("no error was returned, expected BadFilenameError")
    }
}
func TestCreateFileAlreadyExists(t *testing.T) {
    fmt.Println("creating file that already exists")
    err := rfs.CreateFile("test")
    if err != nil {
        fmt.Println("error returned: ", err)
    } else {
        t.Errorf("no error was returned, expected FileExistsError")
    }
}
func TestCreateFileDisconnected(t *testing.T) {

}
func TestCreateFileTwice(t *testing.T) {

    fmt.Println("creating two identical files in quick succession")

    go createFileTwiceHelper()
    time.Sleep(1)
    err := createFileTwiceHelper()
    if err != nil {
        t.Errorf("this test's result doesn't matter")
    } else {
        fmt.Println("both clients had their request added")
    }
}

func createFileTwiceHelper() error {
    fmt.Println("creating file")
    err := rfs.CreateFile("test3")
    if err != nil {
        return err
    }
    fmt.Println("creation succeeded")
    return nil
}
func TestAppendFileDoesntExist(t *testing.T) {
    var byteArray [512]byte
    copy(byteArray[:], "test record")
    var testRecord Record = byteArray
    fmt.Println("appending record to file test")

    _, err := rfs.AppendRec("notreal", &testRecord)
    if err != nil {
        fmt.Println("error returned: ", err)
    } else {
        t.Errorf("no error was returned, expected FileDoesntExistsError")
    }
}
func TestAppendFileMaxLen(t *testing.T) {
}
