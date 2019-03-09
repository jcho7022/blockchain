package minerlib

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
)

type Record [512]byte

type BlockMap struct {
	TailBlock Block
	GenesisBlock Block
	Map map[string]*Block
	mapLock sync.RWMutex
	ChainChange chan []Op
	ProcessingOps []Op
}

type BM interface {
	Insert(block Block) (err error)
	GetLongestChain() ([]Block)
}

var (
	Configs Settings
)

type Block struct{
	PrevHash string
	Ops []Op
	Nonce string
	MinerId string
	Depth int
}

func Initialize(settings Settings, GenesisBlock Block) (blockmap BlockMap){
	Configs = settings
	blockmap = BlockMap{}
	GenesisBlock.Depth = 0
	blockmap.TailBlock = GenesisBlock
	blockmap.GenesisBlock = GenesisBlock
	blockmap.Map = make(map[string]*Block)
	blockmap.mapLock = sync.RWMutex{}
	blockmap.Map[GetHash(GenesisBlock)] = &GenesisBlock

	blockmap.ChainChange = make(chan []Op)
	return blockmap
}

////////////////////////////////////////////////////////////////////////////////////////////
// <ERROR DEFINITIONS>

// These type definitions allow the application to explicitly check
// for the kind of error that occurred. Each API call below lists the
// errors that it is allowed to raise.
//
// Also see:
// https://blog.golang.org/error-handling-and-go
// https://blog.golang.org/errors-are-values

// Contains minerAddr
type DisconnectedError string

func (e DisconnectedError) Error() string {
	return fmt.Sprintf("RFS: Disconnected from the miner [%s]", string(e))
}

// Contains filename. The *only* constraint on filenames in RFS is
// that must be at most 64 bytes long.
type BadFilenameError string

func (e BadFilenameError) Error() string {
	return fmt.Sprintf("RFS: Filename [%s] has the wrong length", string(e))
}

// Contains filename
type FileDoesNotExistError string

func (e FileDoesNotExistError) Error() string {
	return fmt.Sprintf("RFS: Cannot open file [%s] in D mode as it does not exist locally", string(e))
}

// Contains filename
type FileExistsError string

func (e FileExistsError) Error() string {
	return fmt.Sprintf("RFS: Cannot create file with filename [%s] as it already exists", string(e))
}

// Contains filename
type FileMaxLenReachedError string

func (e FileMaxLenReachedError) Error() string {
	return fmt.Sprintf("RFS: File [%s] has reached its maximum length", string(e))
}

type RecordIndexOutOfBoundError string

func (e RecordIndexOutOfBoundError) Error() string {
	return fmt.Sprintf("Block-Map: Error Record out of bound error for record [%s]", string(e))
}


type PrevHashDoesNotExistError string

func (e PrevHashDoesNotExistError) Error() string {
	return fmt.Sprintf("Block-Map: Error hash does not exist in map [%s]", string(e))
}

type BlockNotValidError string

func (e BlockNotValidError) Error() string {
	return fmt.Sprintf("Block-Map: Error block does not end with continuous 0s [%s]", string(e))
}
// </ERROR DEFINITIONS>
////////////////////////////////////////////////////////////////////////////////////////////


// Gets the hash of the block
func GetHash(block Block) string{
	h := md5.New()
	h.Write([]byte(fmt.Sprintf("%v", block)))
	return hex.EncodeToString(h.Sum(nil))
}

func (bm *BlockMap) CheckIfExists(prevHash string) bool{
	bm.mapLock.Lock()
        defer bm.mapLock.Unlock()
	_, ok := bm.Map[prevHash]
	return ok
}

func (bm *BlockMap) GetBlock(hash string) *Block {
	bm.mapLock.Lock()
        defer bm.mapLock.Unlock()
	return bm.Map[hash]

}

// Inserts a block in the block map
// Precondition Block should be valid that is all fields should be filled
// and the the previous hash should exist
// also the hash of the block should end with some number of 0s
func (bm *BlockMap) Insert(block Block) (err error){
	err = bm.ValidateOps(block.Ops)
	if err != nil {
		return err
	}

	if block.Ops != nil && len(block.Ops) != 0 && !BHashEndsWithZeros(block, Configs.PowPerOpBlock) {
		return BlockNotValidError(GetHash(block))
	}
	if len(block.Ops) ==  0 && !BHashEndsWithZeros(block, Configs.PowPerNoOpBlock) {
		return BlockNotValidError(GetHash(block))
	}
	if block.Ops != nil && len(block.Ops) != 0 {
		//fmt.Println("Op block added successfully:", block.PrevHash)
	}
	bm.mapLock.Lock()
	defer bm.mapLock.Unlock()
	if _, ok := bm.Map[block.PrevHash]; ok {
		bm.Map[GetHash(block)] = &block
		bm.updateLongest(block)
		//fmt.Println("prev hash: ", block.PrevHash)
		//fmt.Println("minerID: ", block.MinerId)
		//fmt.Println("height: ", block.Depth)
		//fmt.Println("num ops: ", len(block.Ops))
		return nil
	} else {
		return PrevHashDoesNotExistError(block.PrevHash)
	}
}


// This function must be called everytime client requests touch append operations
func(bm *BlockMap) ValidateOp(op Op) error{
	if op.Op == "append" && !bm.CheckIfFileExists(op.Fname) {
		return FileDoesNotExistError(op.Fname)
	}
	if op.Op == "append" && bm.CheckFileSize(op.Fname) > 65535 {
		return FileMaxLenReachedError(op.Fname)
	}
	if op.Op == "touch" && bm.CheckIfFileExists(op.Fname) {
		return FileExistsError(op.Fname)
	}
	if op.Op == "touch" && len(op.Fname) > 64 {
		return BadFilenameError(op.Fname)
	}
	return nil
}


func(bm *BlockMap) ValidateOps(ops []Op) error{
	for _,op := range ops{
		if op.Op == "append" && !bm.CheckIfFileExists(op.Fname) {
			return FileDoesNotExistError(op.Fname)
		}
		if op.Op == "append" && bm.CheckFileSize(op.Fname) > 65535 {
			return FileMaxLenReachedError(op.Fname)
		}
		if op.Op == "touch" && bm.CheckIfFileExists(op.Fname) {
			return FileExistsError(op.Fname)
		}
		if op.Op == "touch" && len(op.Fname) > 64 {
			return BadFilenameError(op.Fname)
		}
	}
	return nil
}

func (bm *BlockMap) updateLongest(block Block) {
	if block.Depth == bm.TailBlock.Depth {
		if rand.Intn(2) == 1 {
			bm.TailBlock = block
			bm.ChainChange <- bm.ProcessingOps
		}
	}
	if block.Depth > bm.TailBlock.Depth {
		bm.TailBlock = block
		bm.ChainChange <- bm.ProcessingOps
	}
}


func BHashEndsWithZeros(block Block, numZeros uint8) bool{
	hash := GetHash(block)
	for i:= len(hash) - 1; i > len(hash) - int(numZeros) ; i--{
		if hash[i] != '0' {
			return false
		}
	}
	return true
}

func (bm *BlockMap) SetTailBlock(block Block){
	bm.TailBlock = block
}

// Mines a no op block and puts it in the block chain
// returns nil to the channel if invalid block
func (bm *BlockMap) MineAndAddNoOpBlock(minerId string, blockCh chan Block){
	bm.ProcessingOps = nil
	block := Block{ PrevHash: GetHash(bm.TailBlock),
		MinerId:minerId,
		Depth: bm.TailBlock.Depth+1}
	var minedBlock *Block
	minedBlock = ComputeBlock(block , Configs.PowPerNoOpBlock)
	if minedBlock != nil {
		bm.Insert(*minedBlock)
		blockCh <- *minedBlock
	}
}

// Mines an op block and puts it in the block chain
// returns nil to the channel if invalid block
func (bm *BlockMap) MineAndAddOpBlock(ops []Op, minerId string, blockCh chan Block){
	fmt.Println("creating op block")
	block := Block{ PrevHash: GetHash(bm.TailBlock),
		MinerId:minerId,
		Depth: bm.TailBlock.Depth+1}
	var minedBlock *Block
	var validatedOps []Op
	tempFiles := make(map[string]bool)
	for _,op := range ops{
		if bm.CheckIfOpIsValid(op) && !tempFiles[op.Fname]{
			if(op.Op == "touch"){
				tempFiles[op.Fname] = true
			}
			// fmt.Println("Op being validated: ", op.SeqNum)
			validatedOps = append(validatedOps,op)
		}
	}
	//fmt.Println("validated ops:", validatedOps)
	//fmt.Println("validated ops including balance:", bm.validateOpsBalance(validatedOps))
	block.Ops = validatedOps
	bm.ProcessingOps = validatedOps
	minedBlock = ComputeBlock(block , Configs.PowPerOpBlock)
	if minedBlock != nil {
		bm.Insert(*minedBlock)
		fmt.Println("Block mined: ", *minedBlock)
		blockCh <- *minedBlock
	}
}

func (bm *BlockMap) validateOpsBalance(ops []Op) []Op{
	price := make(map[string]int)
        for _,op := range ops{
                if _, ok := price[op.MinerId]; !ok {
                        price[op.MinerId] = 0
                }
                if(op.Op == "touch"){
                        price[op.MinerId] += int(Configs.NumCoinsPerFileCreate)
                } else{
                        price[op.MinerId] += 1
                }
        }

	var validatedCostOps []Op
        for miner,coins := range price{
                minerCoins := bm.CountCoins(miner)
                if(minerCoins < coins){
                        for _,op := range ops{
                                if op.MinerId == miner {
                                        if minerCoins < coins {
                                                if op.Op == "touch" {
                                                        coins -= int(Configs.NumCoinsPerFileCreate)
                                                } else {
                                                        coins -= 1
                                                }
                                                continue
                                        }
                                }
                                    validatedCostOps = append(validatedCostOps,op)
                            }
                } else {
                        for _,op := range ops{
                                if(op.MinerId == miner) {
                                        validatedCostOps = append(validatedCostOps,op)
                                }
                        }
                }
        }
	for _,op := range validatedCostOps{
		fmt.Println(op.SeqNum)
	}
	return validatedCostOps
}



// gets the longest chain of the blockchain with the first array to be the most recent
// block in the map and the last to be the genesis block
func (bm *BlockMap) GetLongestChain() ([]Block){
	var blockChain []Block
	var currBlock = bm.TailBlock
	bm.mapLock.Lock()
        defer bm.mapLock.Unlock()
	for currBlock.PrevHash != bm.GenesisBlock.PrevHash {
		blockChain = append(blockChain, currBlock)
		currBlock = *(bm.Map[currBlock.PrevHash])
	}
	blockChain = append(blockChain, bm.GenesisBlock)
	return blockChain
}

func (bm *BlockMap) LS() map[string]int{
	bc := bm.GetLongestChain()
	fs := make(map[string]int)
	for i := len(bc)-1 ; i >= 0 ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				switch op.Op{
				case "append":
					if _, ok := fs[op.Fname]; ok {
						if int(Configs.ConfirmsPerFileAppend) <= i {
							fs[op.Fname]++
						}
					}
				case "touch":
					if int(Configs.ConfirmsPerFileCreate) <= i {
						fs[op.Fname] = 0
					}
				}
			}
		}
	}
	return fs
}

func (bm *BlockMap) Cat(fname string) []Record{
	bc := bm.GetLongestChain()
	var f []Record
	for i := len(bc)-1 ; i >= int(Configs.ConfirmsPerFileAppend) ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.Op == "append" && op.Fname == fname {
					f = append(f, op.Rec)
				}
			}
		}
	}
	return f
}

func (bm *BlockMap) Tail(k int,fname string) []Record{
	bc := bm.GetLongestChain()
	var f []Record
	for i := int(Configs.ConfirmsPerFileAppend) ; i < len(bc) ; i++{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for n := len(bc[i].Ops) -1 ; n >= 0 ; n--{
				op := bc[i].Ops[n]
				if op.Op == "append" && op.Fname == fname {
					f = append(f, op.Rec)
				}
				if len(f) == k {
					return reverse(f)
				}
			}
		}
	}
	return reverse(f)
}

func reverse(l []Record) []Record{
	var revList []Record
	for i:=len(l)-1 ; i>= 0; i--{
		revList = append(revList, l[i])
	}
	return revList
}

func (bm *BlockMap) Head(k int,fname string) []Record{
	bc := bm.GetLongestChain()
	var f []Record
	for i := len(bc)-1 ; i >= int(Configs.ConfirmsPerFileAppend) ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.Op == "append" && op.Fname == fname {
					f = append(f, op.Rec)
				}
				if len(f) == k {
					return f
				}
			}
		}
	}
	return f
}

func (bm *BlockMap) CountCoins(minerId string) int{
	bc := bm.GetLongestChain()
	var coins = 0
	var appends int
	var touches int
	for i := len(bc) - 1 ; i >= 0 ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			appends = 0
			touches = 0
			for _,op := range bc[i].Ops{
				switch op.Op{
				case "append":
					if int(Configs.ConfirmsPerFileAppend) <= i {
						if op.MinerId == minerId {
							appends++
						}
					}
				case "touch":
					if int(Configs.ConfirmsPerFileCreate) <= i {
						if op.MinerId == minerId {
							touches++
						}
					}
				}
			}
			// fmt.Println("appends:", appends)
			// fmt.Println("touches:", touches)
			if bc[i].MinerId == minerId {
				coins += int(Configs.MinedCoinsPerOpBlock)
			}
			coins = coins - appends - touches*int(Configs.NumCoinsPerFileCreate)
			// fmt.Println("coins:", coins)
		} else {
			if bc[i].MinerId == minerId {
				coins += int(Configs.MinedCoinsPerNoOpBlock)
			}
		}
	}
	return coins
}

func (bm *BlockMap) CheckIfFileExists(fname string) bool{
	bc := bm.GetLongestChain()
	for i := len(bc)-1 ; i >= int(Configs.ConfirmsPerFileCreate) ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.Op == "touch" && op.Fname == fname {
					return true
				}
			}
		}
	}
	return false
}

// num records in a file
func (bm *BlockMap) CheckFileSize(fname string) int{
	bc := bm.GetLongestChain()
	size := 0
	for i := len(bc)-1 ; i >= int(Configs.ConfirmsPerFileAppend) ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.Op == "append" && op.Fname == fname {
					size ++
				}
			}
		}
	}
	return size
}

func (bm *BlockMap) CheckIfOpExists(seqNum int) bool{
	bc := bm.GetLongestChain()
	for i := len(bc)-1 ; i >= 0 ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.SeqNum == seqNum {
					return true
				}
			}
		}
	}
	return false
}

func (bm *BlockMap) GetRecordPosition(seqNum int, fname string) int{
	bc := bm.GetLongestChain()
	numRecord := 0
	for i := len(bc)-1 ; i >= int(Configs.ConfirmsPerFileAppend) ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.SeqNum == seqNum {
					return numRecord
				}
				if op.Op == "append" && op.Fname == fname {
					numRecord++
				}
			}
		}
	}
	return -1
}

func (bm *BlockMap) GetRecAtPosition(fname string, reqNum int) (rec Record, err error){
	bc := bm.GetLongestChain()
	index := 0
	for i := len(bc)-1 ; i >= int(Configs.ConfirmsPerFileAppend) ; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.Op == "append" && op.Fname == fname && index == reqNum {
					return op.Rec, nil
				}
				if op.Op == "append" && op.Fname == fname {
					index++
				}
			}
		}
	}
	return rec, RecordIndexOutOfBoundError(fname)
}

func (bm *BlockMap) CheckIfOpIsValid(operation Op) bool{
	if len(operation.Fname) > 64 {
            return false
        }

	bc := bm.GetLongestChain()
	size := 0
	for i := len(bc)-1 ; i >= 0; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.Op == "touch" && operation.Op == "touch" && op.Fname == operation.Fname && op.SeqNum != operation.SeqNum && i >= int(Configs.ConfirmsPerFileCreate) {
					return false
				}
				if op.SeqNum == operation.SeqNum {
					return false
				}
				if op.Op == "append" && op.Fname == operation.Fname {
					size ++
				}
				if size > 65535 {
					return false
				}
			}
		}
	}
	return true
}

func (bm *BlockMap) CheckIfOpIsConfirmed(operation Op) int {
	bc := bm.GetLongestChain()
	var reqBlocksForConfirms int
	if operation.Op  == "touch" {
		reqBlocksForConfirms = int(Configs.ConfirmsPerFileCreate)
	} else{
		reqBlocksForConfirms = int(Configs.ConfirmsPerFileAppend)
	}
	for i := len(bc)-1 ; i >= reqBlocksForConfirms; i--{
		if bc[i].Ops != nil && len(bc[i].Ops) != 0 {
			for _,op := range bc[i].Ops{
				if op.Op == "touch" && operation.Op == "touch" && op.Fname == operation.Fname && op.SeqNum == operation.SeqNum {
					return 1
				}
				if op.Op == "touch" && operation.Op == "touch" && op.Fname == operation.Fname && op.SeqNum != operation.SeqNum {
					return -1
				}
				if op.Op == "append" && operation.Op == "append" &&  op.Fname == operation.Fname && op.SeqNum == operation.SeqNum {
					return 1
				}
			}
		}
	}
	return 0
}



