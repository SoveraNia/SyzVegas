// Copyright 2018 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// MODIFIED: Daimeng Wang

package main

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/google/syzkaller/pkg/cover"
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/rpctype"
	"github.com/google/syzkaller/pkg/signal"
	"github.com/google/syzkaller/prog"
	"github.com/google/syzkaller/sys"
)

type RPCServer struct {
	mgr             RPCManagerView
	target          *prog.Target
	enabledSyscalls []int
	stats           *Stats
	sandbox         string
	batchSize       int

	mu           sync.Mutex
	fuzzers      map[string]*Fuzzer
	checkResult  *rpctype.CheckArgs
	maxSignal    signal.Signal
	corpusSignal signal.Signal
	corpusCover  cover.Cover
	rotator      *prog.Rotator
	rnd          *rand.Rand

	triageWorks map[hash.Sig]map[int]rpctype.RPCTriage

	MABRound      int
	MABExp31Round int
	MABGLC        rpctype.MABGLC

	MABTriageCount      int
	MABTriageCostBefore float64
	MABTriageCostAfter  float64

	CorpusGLC  map[hash.Sig]rpctype.CorpusGLC
	TriageInfo map[hash.Sig]*rpctype.TriageInfo
}

type Fuzzer struct {
	name          string
	inputs        []rpctype.RPCInput
	newMaxSignal  signal.Signal
	rotatedSignal signal.Signal
	triages       []rpctype.RPCTriage
}

type BugFrames struct {
	memoryLeaks []string
	dataRaces   []string
}

// RPCManagerView restricts interface between RPCServer and Manager.
type RPCManagerView interface {
	fuzzerConnect() ([]rpctype.RPCInput, BugFrames)
	machineChecked(result *rpctype.CheckArgs)
	newInput(inp rpctype.RPCInput, sign signal.Signal)
	candidateBatch(size int) []rpctype.RPCCandidate
	rotateCorpus() bool
}

func startRPCServer(mgr *Manager) (int, error) {
	serv := &RPCServer{
		mgr:             mgr,
		target:          mgr.target,
		enabledSyscalls: mgr.enabledSyscalls,
		stats:           mgr.stats,
		sandbox:         mgr.cfg.Sandbox,
		fuzzers:         make(map[string]*Fuzzer),
		rnd:             rand.New(rand.NewSource(time.Now().UnixNano())),
		MABRound:        0,
		MABGLC:          rpctype.MABGLC{},
		CorpusGLC:       make(map[hash.Sig]rpctype.CorpusGLC),
		triageWorks:     make(map[hash.Sig]map[int]rpctype.RPCTriage),
		TriageInfo:      make(map[hash.Sig]*rpctype.TriageInfo),
	}
	serv.batchSize = 5
	if serv.batchSize < mgr.cfg.Procs {
		serv.batchSize = mgr.cfg.Procs
	}
	s, err := rpctype.NewRPCServer(mgr.cfg.RPC, "Manager", serv)
	if err != nil {
		return 0, err
	}
	log.Logf(0, "serving rpc on tcp://%v", s.Addr())
	port := s.Addr().(*net.TCPAddr).Port
	go s.Serve()
	return port, nil
}

func (serv *RPCServer) Connect(a *rpctype.ConnectArgs, r *rpctype.ConnectRes) error {
	log.Logf(1, "fuzzer %v connected", a.Name)
	serv.stats.vmRestarts.inc()

	corpus, bugFrames := serv.mgr.fuzzerConnect()

	serv.mu.Lock()
	defer serv.mu.Unlock()

	f := &Fuzzer{
		name: a.Name,
	}
	serv.fuzzers[a.Name] = f
	r.MemoryLeakFrames = bugFrames.memoryLeaks
	r.DataRaceFrames = bugFrames.dataRaces
	r.EnabledCalls = serv.enabledSyscalls
	r.GitRevision = sys.GitRevision
	r.TargetRevision = serv.target.Revision
	// TODO: temporary disabled b/c we suspect this negatively affects fuzzing.
	if false && serv.mgr.rotateCorpus() && serv.rnd.Intn(3) != 0 {
		// We do rotation every other time because there are no objective
		// proofs regarding its efficiency either way.
		// Also, rotation gives significantly skewed syscall selection
		// (run prog.TestRotationCoverage), it may or may not be OK.
		r.CheckResult = serv.rotateCorpus(f, corpus)
	} else {
		r.CheckResult = serv.checkResult
		f.inputs = corpus
		f.newMaxSignal = serv.maxSignal.Copy()
	}
	return nil
}

func (serv *RPCServer) rotateCorpus(f *Fuzzer, corpus []rpctype.RPCInput) *rpctype.CheckArgs {
	// Fuzzing tends to stuck in some local optimum and then it fails to cover
	// other state space points since code coverage is only a very approximate
	// measure of logic coverage. To overcome this we introduce some variation
	// into the process which should cause steady corpus rotation over time
	// (the same coverage is achieved in different ways).
	//
	// First, we select a subset of all syscalls for each VM run (result.EnabledCalls).
	// This serves 2 goals: (1) target fuzzer at a particular area of state space,
	// (2) disable syscalls that cause frequent crashes at least in some runs
	// to allow it to do actual fuzzing.
	//
	// Then, we remove programs that contain disabled syscalls from corpus
	// that will be sent to the VM (f.inputs). We also remove 10% of remaining
	// programs at random to allow to rediscover different variations of these programs.
	//
	// Then, we drop signal provided by the removed programs and also 10%
	// of the remaining signal at random (f.newMaxSignal). This again allows
	// rediscovery of this signal by different programs.
	//
	// Finally, we adjust criteria for accepting new programs from this VM (f.rotatedSignal).
	// This allows to accept rediscovered varied programs even if they don't
	// increase overall coverage. As the result we have multiple programs
	// providing the same duplicate coverage, these are removed during periodic
	// corpus minimization process. The minimization process is specifically
	// non-deterministic to allow the corpus rotation.
	//
	// Note: at no point we drop anything globally and permanently.
	// Everything we remove during this process is temporal and specific to a single VM.
	calls := serv.rotator.Select()

	var callIDs []int
	callNames := make(map[string]bool)
	for call := range calls {
		callNames[call.Name] = true
		callIDs = append(callIDs, call.ID)
	}

	f.inputs, f.newMaxSignal = serv.selectInputs(callNames, corpus, serv.maxSignal)
	// Remove the corresponding signal from rotatedSignal which will
	// be used to accept new inputs from this manager.
	f.rotatedSignal = serv.corpusSignal.Intersection(f.newMaxSignal)

	result := *serv.checkResult
	result.EnabledCalls = map[string][]int{serv.sandbox: callIDs}
	return &result
}

func (serv *RPCServer) selectInputs(enabled map[string]bool, inputs0 []rpctype.RPCInput, signal0 signal.Signal) (
	inputs []rpctype.RPCInput, signal signal.Signal) {
	signal = signal0.Copy()
	for _, inp := range inputs0 {
		calls, err := prog.CallSet(inp.Prog)
		if err != nil {
			panic(fmt.Sprintf("rotateInputs: CallSet failed: %v\n%s", err, inp.Prog))
		}
		for call := range calls {
			if !enabled[call] {
				goto drop
			}
		}
		if serv.rnd.Float64() > 0.9 {
			goto drop
		}
		inputs = append(inputs, inp)
		continue
	drop:
		for _, sig := range inp.Signal.Elems {
			delete(signal, sig)
		}
	}
	signal.Split(len(signal) / 10)
	return inputs, signal
}

func (serv *RPCServer) Check(a *rpctype.CheckArgs, r *int) error {
	serv.mu.Lock()
	defer serv.mu.Unlock()

	if serv.checkResult != nil {
		return nil
	}
	serv.mgr.machineChecked(a)
	a.DisabledCalls = nil
	serv.checkResult = a
	calls := make(map[*prog.Syscall]bool)
	for _, call := range a.EnabledCalls[serv.sandbox] {
		calls[serv.target.Syscalls[call]] = true
	}
	serv.rotator = prog.MakeRotator(serv.target, calls, serv.rnd)
	return nil
}

func (serv *RPCServer) SyncMABStatus(a *rpctype.RPCMABStatus, r *rpctype.RPCMABStatus) error {
	if a.Round > serv.MABRound {
		serv.MABRound = a.Round
		serv.MABExp31Round = a.Exp31Round
		// serv.MABMaxGain = a.MaxGain
		// serv.MABMinGain = a.MinGain
		// serv.MABMaxLoss = a.MaxLoss
		// serv.MABMinLoss = a.MinLoss
		// serv.MABMaxCost = a.MaxCost
		// serv.MABMinCost = a.MinCost
		serv.MABGLC = a.MABGLC
		// serv.MABWindowGain = append([]float64(nil), a.WindowGain...)
		// serv.MABWindowLoss = append([]float64(nil), a.WindowLoss...)
		// serv.MABWindowCost = append([]float64(nil), a.WindowCost...)
		for sig, v := range a.CorpusGLC {
			serv.CorpusGLC[sig] = v
			log.Logf(4, "- MAB Corpus Sync %v: %+v\n", sig.String(), v)
		}
		for sig, v := range a.TriageInfo {
			if v.TriageCount >= v.TriageTotal {
				delete(serv.TriageInfo, sig)
				log.Logf(4, "Deleting completed triage %v", sig.String())
			} else {
				serv.TriageInfo[sig] = &rpctype.TriageInfo{
					Source:           v.Source,
					SourceCost:       v.SourceCost,
					TriageGain:       v.TriageGain,
					VerifyGain:       v.VerifyGain,
					VerifyCost:       v.VerifyCost,
					MinimizeGain:     v.MinimizeGain,
					MinimizeCost:     v.MinimizeCost,
					MinimizeTimeSave: v.MinimizeTimeSave,
					TriageCount:      v.TriageCount,
					TriageTotal:      v.TriageTotal,
					TriageGainNorm:   v.TriageGainNorm,
					SourceGainNorm:   v.SourceGainNorm,
				}
			}
		}
	} else if a.Round < serv.MABRound {
		r.Round = serv.MABRound
		r.Exp31Round = serv.MABExp31Round
		r.MABGLC = serv.MABGLC
		r.CorpusGLC = make(map[hash.Sig]rpctype.CorpusGLC)
		for sig, v := range serv.CorpusGLC {
			r.CorpusGLC[sig] = v
		}
		r.TriageInfo = make(map[hash.Sig]*rpctype.TriageInfo)
		for sig, v := range serv.TriageInfo {
			r.TriageInfo[sig] = v
		}
	}
	return nil
}

func (serv *RPCServer) NewInput(a *rpctype.NewInputArgs, r *int) error {
	inputSignal := a.Signal.Deserialize()
	log.Logf(4, "new input from %v for syscall %v (signal=%v, cover=%v)",
		a.Name, a.Call, inputSignal.Len(), len(a.Cover))
	if _, err := serv.target.Deserialize(a.RPCInput.Prog, prog.NonStrict); err != nil {
		// This should not happen, but we see such cases episodically, reason unknown.
		log.Logf(0, "failed to deserialize program from fuzzer: %v\n%s", err, a.RPCInput.Prog)
		return nil
	}
	serv.mu.Lock()
	defer serv.mu.Unlock()

	f := serv.fuzzers[a.Name]
	genuine := !serv.corpusSignal.Diff(inputSignal).Empty()
	rotated := false
	if !genuine && f.rotatedSignal != nil {
		rotated = !f.rotatedSignal.Diff(inputSignal).Empty()
	}
	if !genuine && !rotated {
		return nil
	}
	serv.mgr.newInput(a.RPCInput, inputSignal)

	// Update corpus GLC
	sig := hash.Hash(a.RPCInput.Prog)
	serv.CorpusGLC[sig] = a.RPCInput.CorpusGLC

	if f.rotatedSignal != nil {
		f.rotatedSignal.Merge(inputSignal)
	}
	serv.corpusCover.Merge(a.Cover)
	serv.stats.corpusCover.set(len(serv.corpusCover))
	serv.stats.newInputs.inc()
	if rotated {
		serv.stats.rotatedInputs.inc()
	}

	if genuine {
		serv.corpusSignal.Merge(inputSignal)
		serv.stats.corpusSignal.set(serv.corpusSignal.Len())

		a.RPCInput.Cover = nil // Don't send coverage back to all fuzzers.
		for _, other := range serv.fuzzers {
			if other == f {
				continue
			}
			other.inputs = append(other.inputs, a.RPCInput)
		}
	}
	return nil
}

func (serv *RPCServer) Poll(a *rpctype.PollArgs, r *rpctype.PollRes) error {
	serv.stats.mergeNamed(a.Stats)

	serv.mu.Lock()
	defer serv.mu.Unlock()

	f := serv.fuzzers[a.Name]
	if f == nil {
		log.Fatalf("fuzzer %v is not connected", a.Name)
	}
	newMaxSignal := serv.maxSignal.Diff(a.MaxSignal.Deserialize())
	if !newMaxSignal.Empty() {
		serv.maxSignal.Merge(newMaxSignal)
		for _, f1 := range serv.fuzzers {
			if f1 == f {
				continue
			}
			f1.newMaxSignal.Merge(newMaxSignal)
		}
	}
	r.MaxSignal = f.newMaxSignal.Split(500).Serialize()
	if a.NeedCandidates {
		r.Candidates = serv.mgr.candidateBatch(serv.batchSize)
	}
	if len(r.Candidates) == 0 {
		batchSize := serv.batchSize
		// When the fuzzer starts, it pumps the whole corpus.
		// If we do it using the final batchSize, it can be very slow
		// (batch of size 6 can take more than 10 mins for 50K corpus and slow kernel).
		// So use a larger batch initially (we use no stats as approximation of initial pump).
		const initialBatch = 30
		if len(a.Stats) == 0 && batchSize < initialBatch {
			batchSize = initialBatch
		}
		for i := 0; i < batchSize && len(f.inputs) > 0; i++ {
			last := len(f.inputs) - 1
			r.NewInputs = append(r.NewInputs, f.inputs[last])

			// Send MAB status as well
			sig := hash.Hash(f.inputs[last].Prog)
			if r.CorpusGLC == nil {
				log.Logf(0, "- WTF. nil map detected. Creating.\n")
				r.CorpusGLC = make(map[hash.Sig]rpctype.CorpusGLC)
			}
			if v, ok := serv.CorpusGLC[sig]; ok {
				r.CorpusGLC[sig] = v
				r.NewInputs[len(r.NewInputs)-1].CorpusGLC = v
				log.Logf(4, "- Sending corpus GLC %v: %+v.\n", sig.String(), r.CorpusGLC[sig])
			}

			f.inputs[last] = rpctype.RPCInput{}
			f.inputs = f.inputs[:last]
		}
		if len(f.inputs) == 0 {
			f.inputs = nil
		}
	}
	// Receive incompleted triages
	for _, v := range a.TriagesUnfinished {
		if _, ok := serv.triageWorks[v.Sig]; !ok {
			serv.triageWorks[v.Sig] = make(map[int]rpctype.RPCTriage)
		}
		if _, ok := serv.triageWorks[v.Sig][v.CallIndex]; ok {
			log.Logf(4, "WTF Duplicate Triage: Prog=%v, Index=%v, #Sig=%v", v.Sig.String(), v.CallIndex, len(v.Info.Signal))
			return nil
		}
		serv.triageWorks[v.Sig][v.CallIndex] = v
		f.triages = append(f.triages, v)
		log.Logf(4, "Fuzzer %v New Triage: Prog=%v, Index=%v, #Sig=%v", a.Name, v.Sig.String(), v.CallIndex, len(v.Info.Signal))
	}
	// Receive completed triages
	for sig, v := range a.Triages {
		if _, ok := serv.triageWorks[sig]; ok && v == 1 {
			delete(serv.triageWorks, sig)
			log.Logf(4, "Triage Complete: Prog=%v", sig.String())
		}
	}
	// Receive completed smashes
	for _, sig := range a.SmashesFinished {
		if _, ok := serv.CorpusGLC[sig]; ok && !serv.CorpusGLC[sig].Smashed {
			tmp := serv.CorpusGLC[sig]
			tmp.Smashed = true
			serv.CorpusGLC[sig] = tmp
			log.Logf(4, "Smash Complete: Prog=%v", sig.String())
		}
	}
	// Check what triages needs to be synced
	triageCount := 0
	if a.NeedTriages {
		for sig, _ := range serv.triageWorks {
			for cidx, _ := range serv.triageWorks[sig] {
				insert := false
				if _, ok := a.Triages[sig]; !ok {
					insert = true
				}
				if insert && triageCount <= serv.batchSize {
					r.Triages = append(r.Triages, serv.triageWorks[sig][cidx])
					triageCount += 1
					if triageCount > serv.batchSize {
						break
					}
				}
			}
			if triageCount > serv.batchSize {
				break
			}
		}
	}
	// Sync MAB
	serv.SyncMABStatus(&a.RPCMABStatus, &r.RPCMABStatus)
	log.Logf(4, "poll from %v: candidates=%v inputs=%v triages=%v maxsignal=%v",
		a.Name, len(r.Candidates), len(r.NewInputs), len(r.Triages), len(r.MaxSignal.Elems))
	return nil
}