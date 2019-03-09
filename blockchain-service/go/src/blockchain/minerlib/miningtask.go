package minerlib

import (
	"fmt"
	"math/rand"
	"time"
)




var (
    letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456790123456790123456790123456790")
    InProgress = false
    ContinueMining = false
)


func randSeq(n int) string {
    b := make([]rune, n)
    for i := range b {
        b[i] = letters[rand.Intn(len(letters))]
    }
    return string(b)
}

// This must be called in main thread before ComputeBlock.
func PrepareMining(){
    ContinueMining = true
}

func ComputeBlock(block Block, numZeros uint8) *Block{
	if(InProgress){
	    fmt.Println("mining still taking place")
	    return nil
	}
	InProgress = true
	zeros := ""
	for i:= 0; i<int(numZeros); i++{
		zeros = zeros+"0"
	}
        rand.Seed(time.Now().UnixNano())
	for ContinueMining{
		block.Nonce = randSeq(30)
		hash := GetHash(block)
		if(hash[len(hash)-int(numZeros):len(hash)] == zeros){
			InProgress = false
			return &block
		}
	}
	InProgress = false
	return nil
}

func WaitUntilMiningIsDone(){
	for InProgress{
	}
}

func StopMining(){
    ContinueMining = false
}
