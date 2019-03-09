package main

import (
	"blockchain/minerlib"
	"encoding/json"
	"fmt"
	"github.com/DistributedClocks/GoVector/govec"
	"github.com/DistributedClocks/GoVector/govec/vrpc"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"sort"
	"time"
)


type M interface {
	MakeKnown(addr string, reply *int) error
	ReceiveOp(operation minerlib.Op, reply *int) error
	ReceiveBlock(payload Payload, reply *int) error
	GetPreviousBlock(prevHash string, block *minerlib.Block) error
}

var (
    GovecOptions = govec.GetDefaultLogOptions()
    miner *Miner
    MinerLogger *govec.GoLog
    Configs minerlib.Settings
    blockStartTime int64
    maxOps = 2
    confirmationCheckDelay time.Duration = 3
    confirmationTimeout = 30

    PendingCoins uint8 = 0
)

type Miner struct {
	Connections map[string]*rpc.Client
	WaitingOps map[int]minerlib.Op
	BlockMap minerlib.BlockMap
	IncomingOps chan []minerlib.Op
}

type Payload struct {
	ReturnAddr string
	Block minerlib.Block
}

type AppendReply struct {
	RecordNum int
	Err error
}

type RecordsReply struct{
    Err     error
    Records []minerlib.Record
}

type TotalRecReply struct{
    Err error
    NumRecords int
}

type ReadRecReqest struct{
    Fname string
    RecordNum uint16
}

type LsReply struct{
    Err   error
    Files []string
}

type ReadRecReply struct{
    Err error
    Rec minerlib.Record
}

// returns 1 if miner is connected else 0
func (miner *Miner) IsConnected(clientId string ,res *string) error {
     // // fmt.Println("client connection id:", clientId)
     // // fmt.Println(len(miner.Connections))
     if len(miner.Connections) != 0 {
        *res = Configs.MinerID
     } else{
        *res = "disconnected"
     }

     return nil
}


func (miner *Miner) Ls(op minerlib.Op, reply *LsReply) error {
     if len(miner.Connections) == 0 {
	     reply.Err = minerlib.DisconnectedError(Configs.MinerID)
     } else{
	fs := miner.BlockMap.LS()
		 var files []string
	for key := range fs{
            files = append(files, key)
        }
	reply.Files = files
     }

     return nil
}

func (miner *Miner) TotalRecs(fname string, reply *TotalRecReply) error{
    if len(miner.Connections) == 0 {
        reply.Err = minerlib.DisconnectedError(Configs.MinerID)
     } else{
        fs := miner.BlockMap.LS()
	if val, ok := fs[fname]; ok{
	    reply.NumRecords = val
	} else {
	    reply.Err = minerlib.FileDoesNotExistError(fname)
	}
     }
     return nil
}


func (miner *Miner) ReadRec(req ReadRecReqest, reply *ReadRecReply) error {
     if len(miner.Connections) == 0 {
	     // fmt.Println("read rec reply1",reply)
        reply.Err = minerlib.DisconnectedError(Configs.MinerID)
     } else if !miner.BlockMap.CheckIfFileExists(req.Fname) {
	     // fmt.Println("read rec reply2",reply)
	reply.Err = minerlib.FileDoesNotExistError(req.Fname)
     } else{
	flag := true
	for flag {
	rec,err := miner.BlockMap.GetRecAtPosition(req.Fname,int(req.RecordNum))
	    if err == nil {
	        reply.Rec = rec
		flag = false
		// fmt.Println("read rec reply3",reply)
	    }else{
		    // fmt.Println("read rec reply4",reply)
		time.Sleep(1 * time.Second)
	    }
	}
     }
     return nil
}

func (miner *Miner) Touch(op minerlib.Op, reply *error) error {
	op.SeqNum = int(time.Now().UnixNano())
	// fmt.Println("operation sequence number: ", op.SeqNum)

	err := miner.BlockMap.ValidateOp(op)
	if err != nil {
		switch err.(type) {
		case minerlib.FileDoesNotExistError:
			*reply = minerlib.FileDoesNotExistError(op.SeqNum)
		case minerlib.FileExistsError:
			*reply = minerlib.FileExistsError(op.SeqNum)
		case minerlib.BadFilenameError:
			*reply = minerlib.BadFilenameError(op.SeqNum)
		}
		return nil
	}

	for uint8(miner.BlockMap.CountCoins(Configs.MinerID)) < PendingCoins + Configs.NumCoinsPerFileCreate {
		time.Sleep(1)
		fmt.Println("Not enough coins right now, waiting")
	}
	PendingCoins += Configs.NumCoinsPerFileCreate

	miner.ReceiveOp(op, nil)

	startTime := time.Now().Unix()
	for true {
		time.Sleep(confirmationCheckDelay * time.Second)
		fmt.Println("checking if op added")
		switch {

		case int64(confirmationTimeout) < time.Now().Unix() - startTime:
			fmt.Println("timeout: sending touch operation again")
			miner.ReceiveOp(op, nil)
			startTime = time.Now().Unix()

		default:
			if miner.BlockMap.CheckIfOpIsConfirmed(op) == 1 {
				PendingCoins -= Configs.NumCoinsPerFileCreate
				return nil
			}
			if miner.BlockMap.CheckIfOpIsConfirmed(op) == -1 {
				PendingCoins -= Configs.NumCoinsPerFileCreate
				return minerlib.FileExistsError(op.SeqNum)
			}
		}
	}
	PendingCoins -= Configs.NumCoinsPerFileCreate
	return nil
}

func (miner *Miner) Append(op minerlib.Op, reply *AppendReply) error {
	op.SeqNum = int(time.Now().UnixNano())

	err := miner.BlockMap.ValidateOp(op)
	if err != nil {
		fmt.Println(err)
		switch err.(type) {
		case minerlib.FileDoesNotExistError:
			*reply = AppendReply{-1, minerlib.FileDoesNotExistError(op.SeqNum)}
		case minerlib.FileExistsError:
			*reply = AppendReply{-1, minerlib.FileExistsError(op.SeqNum)}
		case minerlib.BadFilenameError:
			*reply = AppendReply{-1, minerlib.BadFilenameError(op.SeqNum)}
		case minerlib.FileMaxLenReachedError:
			*reply = AppendReply{-1, minerlib.FileMaxLenReachedError(op.SeqNum)}
		}
		return nil
	}

	for uint8(miner.BlockMap.CountCoins(Configs.MinerID)) < PendingCoins + 1 {
		time.Sleep(1)
		fmt.Println("Not enough coins right now, waiting")
	}
	PendingCoins += 1


	miner.ReceiveOp(op, nil)

	startTime := time.Now().Unix()
	for true {
		time.Sleep(confirmationCheckDelay * time.Second)
		fmt.Println("checking if op added")
		switch {

		case int64(confirmationTimeout) < time.Now().Unix() - startTime:
			fmt.Println("timeout: sending append operation again")
			miner.ReceiveOp(op, nil)
			startTime = time.Now().Unix()

		default:
			if miner.BlockMap.CheckIfOpIsConfirmed(op) == 1 {
				// fmt.Println("Append op confirmed")
				*reply = AppendReply{miner.BlockMap.GetRecordPosition(op.SeqNum, op.Fname), nil}
				PendingCoins -= 1
				return nil
			}
		}
	}
	PendingCoins -= 1
	return nil
}


func (miner *Miner) MakeKnown(addr string, reply *int) error {
	if _, ok := miner.Connections[addr]; !ok {
		MinerLogger.LogLocalEvent("MakeKnown Called", GovecOptions)
		client, err := vrpc.RPCDial("tcp", addr, MinerLogger, GovecOptions)
		if err == nil {
			miner.Connections[addr] = client
			// fmt.Println("connecting client addr: " + addr)
		} else {
			log.Println("dialing:", err)
		}
	}
	return nil
}

func (miner *Miner) ReceiveOp(operation minerlib.Op, reply *int) error {

	if _, ok := miner.WaitingOps[operation.SeqNum]; !ok {
		// op not received yet, store and flood it
		miner.WaitingOps[operation.SeqNum] = operation

		for _, conn := range miner.Connections {
			conn.Go("Miner.ReceiveOp", operation, nil, nil)
		}

		// reset timeout counter
		if blockStartTime == -1 {
			blockStartTime = time.Now().Unix()
		}

		// block timeout or block full, send what you have
		if Configs.GenOpBlockTimeout < uint8(time.Now().Unix() - blockStartTime) || len(miner.WaitingOps) >= maxOps {
			// fmt.Println("Operations added to new Op block")
			blockStartTime = -1
			var newOps []minerlib.Op
			for _, o := range miner.WaitingOps {
				newOps = append(newOps, o)
			}

			sort.Slice(newOps, func(i, j int) bool {
				return newOps[i].SeqNum < newOps[j].SeqNum
			})

			miner.IncomingOps <- newOps
		}
	}
	return nil
}

func (miner *Miner) ReceiveBlock(payload Payload, reply *int) error{
	// if miner is behind, get previous BlockMap.Map until caught up
	ok := miner.BlockMap.CheckIfExists(payload.Block.PrevHash);
	if !ok {
		fmt.Println("recieve block:", payload.Block.Depth)
		fmt.Println("recieve block:", payload.Block.MinerId)
		fmt.Println("prev hash does not exist")
		// add missing BlockMap.Map to a temp store in case they're fake
		missingBlocks := make([]minerlib.Block, 0)
		missingBlocks = append( missingBlocks, payload.Block)

		var prevBlock *minerlib.Block
		prevBlock = &payload.Block
		for !ok {
			miner.Connections[payload.ReturnAddr].Call("Miner.GetPreviousBlock", prevBlock.PrevHash, &prevBlock)
			missingBlocks = append(missingBlocks, *prevBlock)

			ok = miner.BlockMap.CheckIfExists(prevBlock.PrevHash)
		}
		fmt.Println("size",len(missingBlocks))

		// insert all the blocks in missingBlocks into the blockmap
		for i := range missingBlocks {
			fmt.Println("Inserting missing blocks")
			//err := miner.BlockMap.ValidateOps(missingBlocks[len(missingBlocks)-i-1].Ops)
			if true {
				fmt.Println("Inserting missing blocks")
				miner.BlockMap.Insert(missingBlocks[len(missingBlocks)-i-1])
			} else {
				missingBlocks = make([]minerlib.Block, 0)
				break
			}
		}
		for _, conn := range miner.Connections {
                                        conn.Go("Miner.ReceiveBlock", Payload{Configs.IncomingMinersAddr, payload.Block}, nil, nil)
                }


	} else if ok := miner.BlockMap.CheckIfExists(minerlib.GetHash(payload.Block)); !ok {
		// fmt.Println("block received from other miner")

		err := miner.BlockMap.ValidateOps(payload.Block.Ops)
		if err == nil {
			miner.BlockMap.Insert(payload.Block)
			for _, conn := range miner.Connections {
				conn.Go("Miner.ReceiveBlock", Payload{Configs.IncomingMinersAddr, payload.Block}, nil, nil)
			}
		}
		// // fmt.Println("current block chain state:",miner.BlockMap.GetLongestChain())
		// send the block to connected miners
		// // fmt.Println(miner.Connections)
	}

	return nil
}

func (miner *Miner) GetPreviousBlock(prevHash string, block *minerlib.Block) error {

	if ok := miner.BlockMap.CheckIfExists(prevHash); ok {
		block = miner.BlockMap.GetBlock(prevHash)
	}
	return nil
}

func rpcServer() {
    miner = new(Miner)
    MinerLogger = govec.InitGoVector(Configs.MinerID, "./logs/minerlogfile" + Configs.MinerID, govec.GetDefaultConfig())
    miner.BlockMap = minerlib.Initialize(Configs, minerlib.Block{ PrevHash: "GENESIS", Nonce:"GENESIS" , MinerId:"GENESIS"})
    miner.WaitingOps = make(map[int]minerlib.Op)
    miner.Connections = make(map[string]*rpc.Client)
    miner.IncomingOps = make(chan []minerlib.Op)
    server := rpc.NewServer()
    server.Register(miner)
    l, e := net.Listen("tcp", Configs.IncomingMinersAddr)
    if e != nil {
        log.Fatal("listen error:", e)
    }
    vrpc.ServeRPCConn(server, l, MinerLogger, GovecOptions)
}

func rpcClient(){
    for _, addr := range Configs.PeerMinersAddrs {
        client, err := vrpc.RPCDial("tcp", addr, MinerLogger, GovecOptions)
        if err == nil {
        	// make this miner known to the other miner
	    	var result int
	    	err := client.Go("Miner.MakeKnown", Configs.IncomingMinersAddr, &result, nil)
	    	if err != nil {
				// fmt.Println(err)
			}
	    	miner.Connections[addr] = client
	    	// fmt.Println("address added: ", addr)
        } else {log.Println("dialing:", err)}
    }
}

func handleBlocks () {

	// create a noop block and start mining for a nonce
	completeBlock := make(chan minerlib.Block)
	minerlib.WaitUntilMiningIsDone()
	time.Sleep(10 * time.Millisecond)
	minerlib.PrepareMining()
	go miner.BlockMap.MineAndAddNoOpBlock(Configs.MinerID, completeBlock)

	waitingBlocks := make([][]minerlib.Op, 0)

	opBeingMined := false

	for true {

		// no miners connected, do nothing
		if len(miner.Connections) == 0 {
			time.Sleep( 1 * time.Second)
			// fmt.Println("disconnected")
			continue
		}
		select {
		// receive a newly mined block, flood it and start mining noop
		case cb := <-completeBlock:
			// fmt.Println("mined block received")

			// remove ops in this block from waitingOps
			for sn, o := range miner.WaitingOps {
				for _, ob := range cb.Ops {
					if o == ob {
						delete(miner.WaitingOps, sn)
					}
				}
			}
			for _, conn := range miner.Connections {
				var reply int
				conn.Go("Miner.ReceiveBlock", Payload{Configs.IncomingMinersAddr, cb}, &reply, nil)
			}
			// start on noop or queued op block right away
			if len(waitingBlocks) == 0 {
				minerlib.StopMining()
				time.Sleep(50 * time.Millisecond)
				minerlib.PrepareMining()
				go miner.BlockMap.MineAndAddNoOpBlock(Configs.MinerID, completeBlock)
				opBeingMined = false
			} else {
				minerlib.StopMining()
				time.Sleep(50 * time.Millisecond)
                minerlib.PrepareMining()
				go miner.BlockMap.MineAndAddOpBlock(waitingBlocks[0],Configs.MinerID, completeBlock)
				waitingBlocks = waitingBlocks[1:]
				opBeingMined = true
			}

		// receive a new order to mine a block, select whether this block waits or goes forward
		case ib := <-miner.IncomingOps:
			waitingBlocks = append(waitingBlocks, ib)
			// noop block being mined, stop it and start this op block
			if !opBeingMined {
				minerlib.StopMining()
                                time.Sleep(50 * time.Millisecond)
                                minerlib.PrepareMining()
				// fmt.Println("mining new op block")
				go miner.BlockMap.MineAndAddOpBlock(waitingBlocks[0],Configs.MinerID, completeBlock)
				waitingBlocks = waitingBlocks[1:]
				opBeingMined = true
			}

		case retryOps := <- miner.BlockMap.ChainChange:
			if opBeingMined && retryOps != nil {
				fmt.Println("longest chain changed, retrying op block")
				minerlib.StopMining()
				time.Sleep(50 * time.Millisecond)
				minerlib.PrepareMining()
				go miner.BlockMap.MineAndAddOpBlock(retryOps, Configs.MinerID, completeBlock)
				opBeingMined = true
			}
		}
	}
}


func main() {
    // load json settings
    plan, e := ioutil.ReadFile(os.Args[1])
    if e == nil {
		err := json.Unmarshal(plan, &Configs)
		if err != nil {
	    	log.Fatal("error reading json:", err)
        }
    } else {
        log.Fatal("file error:", e)
    }
    go rpcServer()
    time.Sleep(500 * time.Millisecond)
    go rpcClient()
    time.Sleep(500 * time.Millisecond)

    // send this thread to manage the state machine
	handleBlocks()

}
