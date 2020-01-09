// Copyright 2015 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// Package rpctype contains types of message passed via net/rpc connections
// between various parts of the system.
package rpctype

import (
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/host"
	"github.com/google/syzkaller/pkg/ipc"
	"github.com/google/syzkaller/pkg/signal"
)

type RPCInput struct {
	Call   string
	Prog   []byte
	Signal signal.Serial
	Cover  []uint32
	CorpusGLC
}

type RPCTriage struct {
	Sig        hash.Sig
	CallIndex  int
	Prog       []byte
	Flags      int
	Info       ipc.CallInfo
	Source     int
	SourceCost float64
}

type TriageArgs struct {
	Name string
	RPCTriage
}

type GLC struct {
	Count     int
	TotalGain float64
	// TotalLoss  float64
	TotalCost  float64
	TotalGain2 float64
	// TotalLoss2 float64
	TotalCost2 float64
}

type MABGLC struct {
	// For Scheduling
	NormalizedGenerate GLC // Normalized gain for Generate. Used for weight deciding
	NormalizedMutate   GLC // Normalized gain for Mutate. Used for weight deciding
	NormalizedTriage   GLC // Normalized gain for Triage. Used for weight deciding
	RawAll             GLC // Raw gain/cost for all Gen/Mut/Tri. Used for computing Nael's Normallization
	NaelAll            GLC // Nael-Normalized gain/cost for all Gen/Mut/Tri. Used for normalization
	// For Seed selection
	RawMutate  GLC // Raw gain/cost for mutations. Used for Nael's computation for seed selection
	NaelMutate GLC // Nael-Normalized gain/cost for mutations. Used for normalization
}

type TriageInfo struct {
	Source           int
	SourceCost       float64
	TriageGain       float64
	VerifyGain       float64
	VerifyCost       float64
	MinimizeGain     float64
	MinimizeCost     float64
	MinimizeTimeSave float64
	TriageCount      int
	TriageTotal      int
	SourceGainNorm   float64
	TriageGainNorm   float64
}

type CorpusGLC struct {
	Smashed            bool // Whether this seed has been smashed
	MutateCount        int
	MutateCost         float64 // Total cost of mutating this seed
	MutateGain         float64 // Total gain of mutating this seed
	VerifyGain         float64
	VerifyCost         float64
	MinimizeGain       float64
	MinimizeCost       float64
	MinimizeTimeSave   float64 // Time save due to minimization
	CostBeforeMinimize float64 // Cost before minimization
	MutateGainNorm     float64 // Normalized gain after Nael's
	MutateGainNormOrig float64 // Normalized gain after Nael's
	TriageGainNorm     float64 // Normalized gain after Nael's
}

func (glc *GLC) Update(gain float64, cost float64) {
	const Max = 1.0e+100

	glc.Count += 1
	glc.TotalGain += gain
	glc.TotalGain2 += gain * gain
	glc.TotalCost += cost
	glc.TotalCost2 += cost * cost
	if glc.TotalGain > Max {
		glc.TotalGain = Max
	}
	if glc.TotalGain2 > Max {
		glc.TotalGain2 = Max
	}
	if glc.TotalCost > Max {
		glc.TotalCost = Max
	}
	if glc.TotalCost2 > Max {
		glc.TotalCost2 = Max
	}
}

func (glc *GLC) Remove(gain float64, cost float64) {
	glc.Count -= 1
	glc.TotalGain -= gain
	glc.TotalGain2 -= gain * gain
	glc.TotalCost -= cost
	glc.TotalCost2 -= cost * cost
}

type RPCMABStatus struct {
	Round      int
	Exp31Round int
	MABGLC     MABGLC
	CorpusGLC  map[hash.Sig]CorpusGLC
	TriageInfo map[hash.Sig]*TriageInfo
}

type RPCCandidate struct {
	Prog      []byte
	Minimized bool
	Smashed   bool
}

type ConnectArgs struct {
	Name string
}

type ConnectRes struct {
	EnabledCalls     []int
	GitRevision      string
	TargetRevision   string
	AllSandboxes     bool
	CheckResult      *CheckArgs
	MemoryLeakFrames []string
	DataRaceFrames   []string
}

type CheckArgs struct {
	Name          string
	Error         string
	EnabledCalls  map[string][]int
	DisabledCalls map[string][]SyscallReason
	Features      *host.Features
}

type SyscallReason struct {
	ID     int
	Reason string
}

type NewInputArgs struct {
	Name string
	RPCInput
}

type PollArgs struct {
	Name              string
	NeedCandidates    bool
	MaxSignal         signal.Serial
	Stats             map[string]uint64
	NeedTriages       bool
	Triages           map[hash.Sig]int
	TriagesUnfinished []RPCTriage
	SmashesFinished   []hash.Sig
	RPCMABStatus
}

type PollRes struct {
	Candidates []RPCCandidate
	NewInputs  []RPCInput
	Triages    []RPCTriage
	MaxSignal  signal.Serial
	RPCMABStatus
}

type HubConnectArgs struct {
	// Client/Key are used for authentication.
	Client string
	Key    string
	// Manager name, must start with Client.
	Manager string
	// Manager has started with an empty corpus and requests whole hub corpus.
	Fresh bool
	// Set of system call names supported by this manager.
	// Used to filter out programs with unsupported calls.
	Calls []string
	// Current manager corpus.
	Corpus [][]byte
}

type HubSyncArgs struct {
	// see HubConnectArgs.
	Client     string
	Key        string
	Manager    string
	NeedRepros bool
	// Programs added to corpus since last sync or connect.
	Add [][]byte
	// Hashes of programs removed from corpus since last sync or connect.
	Del []string
	// Repros found since last sync.
	Repros [][]byte
}

type HubSyncRes struct {
	// Set of programs from other managers.
	Progs [][]byte
	// Set of repros from other managers.
	Repros [][]byte
	// Number of remaining pending programs,
	// if >0 manager should do sync again.
	More int
}

type RunTestPollReq struct {
	Name string
}

type RunTestPollRes struct {
	ID     int
	Bin    []byte
	Prog   []byte
	Cfg    *ipc.Config
	Opts   *ipc.ExecOpts
	Repeat int
}

type RunTestDoneArgs struct {
	Name   string
	ID     int
	Output []byte
	Info   []*ipc.ProgInfo
	Error  string
}
