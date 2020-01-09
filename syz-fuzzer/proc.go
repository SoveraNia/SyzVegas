// Copyright 2017 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// MODIFIED: Daimeng Wang

package main

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/syzkaller/pkg/cover"
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/ipc"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/rpctype"
	"github.com/google/syzkaller/pkg/signal"
	"github.com/google/syzkaller/prog"
)

const (
	programLength = 30
)

// Proc represents a single fuzzing process (executor).
type Proc struct {
	fuzzer            *Fuzzer
	pid               int
	env               *ipc.Env
	rnd               *rand.Rand
	execOpts          *ipc.ExecOpts
	execOptsCover     *ipc.ExecOpts
	execOptsComps     *ipc.ExecOpts
	execOptsNoCollide *ipc.ExecOpts
}

type ExecResult struct {
	gainRaw   float64
	time      float64
	timeTotal float64
	calls     int
	count     int
	pidx      int
}

type TriageResult struct {
	corpusGainRaw    float64 // # sigs added to the corpus
	verifyGainRaw    float64 // # of sigs discovered by stablization
	verifyTime       float64
	verifyCalls      int
	minimizeGainRaw  float64 // # of sigs discovered by minimization
	minimizeTime     float64
	minimizeTimeSave float64
	minimizeCalls    int
	source           int
	sourceSig        hash.Sig
	sourceCost       float64
	pidx             int
	success          bool
	timeTotal        float64
}

func newProc(fuzzer *Fuzzer, pid int) (*Proc, error) {
	env, err := ipc.MakeEnv(fuzzer.config, pid)
	if err != nil {
		return nil, err
	}
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(pid)*1e12))
	execOptsNoCollide := *fuzzer.execOpts
	execOptsNoCollide.Flags &= ^ipc.FlagCollide
	execOptsCover := execOptsNoCollide
	execOptsCover.Flags |= ipc.FlagCollectCover
	execOptsComps := execOptsNoCollide
	execOptsComps.Flags |= ipc.FlagCollectComps
	proc := &Proc{
		fuzzer:            fuzzer,
		pid:               pid,
		env:               env,
		rnd:               rnd,
		execOpts:          fuzzer.execOpts,
		execOptsCover:     &execOptsCover,
		execOptsComps:     &execOptsComps,
		execOptsNoCollide: &execOptsNoCollide,
	}
	return proc, nil
}

func (proc *Proc) ProcessItem(item interface{}) (int, interface{}) {
	if item != nil {
		switch item := item.(type) {
		case *WorkTriage:
			{
				proc.fuzzer.writeLog("# %s\n", "WorkTriage")
				res := proc.triageInput(item)
				return 2, res
			}
		case *WorkCandidate:
			{
				proc.fuzzer.writeLog("# %s\n", "WorkCandidate")
				_, res := proc.execute(proc.execOpts, item.p, item.flags, StatCandidate)
				return 0, res
			}
		case *WorkSmash:
			{
				proc.fuzzer.writeLog("# %s\n", "WorkSmash")
				res := proc.smashInput(item)
				return 1, res
			}
		default:
			log.Fatalf("unknown work type: %#v", item)
		}
	}
	return -1, nil
}

func (proc *Proc) DoCandidate() {
	item := proc.fuzzer.workQueue.dequeueType(0, true, true)
	for item != nil {
		proc.fuzzer.writeLog("%s", "# WorkCandidate\n")
		proc.ProcessItem(item)
		item = proc.fuzzer.workQueue.dequeueType(0, true, true)
	}
	return
}

func (proc *Proc) DoGenerate() ExecResult {
	ret := ExecResult{
		gainRaw:   0.0,
		time:      0.0,
		calls:     0,
		pidx:      -1,
		timeTotal: 0.0,
	}
	ts0 := time.Now().UnixNano()
	defer func() {
		ret.timeTotal = float64(time.Now().UnixNano()-ts0) / proc.fuzzer.fuzzerConfig.MABTimeUnit
	}()
	ct := proc.fuzzer.choiceTable
	p := proc.fuzzer.target.Generate(proc.rnd, programLength, ct)
	proc.fuzzer.writeLog("%s", "# Generate\n")
	proc.fuzzer.logProgram(p)
	_, r := proc.execute(proc.execOpts, p, ProgNormal, StatSmash)
	ret.gainRaw += r.gainRaw
	ret.calls += r.calls
	ret.time += r.time
	ret.timeTotal += r.timeTotal
	// }
	return ret
}

func (proc *Proc) DoMutate(count int) ExecResult {
	ret := ExecResult{
		gainRaw:   0.0,
		time:      0.0,
		calls:     0,
		pidx:      -1,
		timeTotal: 0.0,
	}
	ts0 := time.Now().UnixNano()
	defer func() {
		ret.timeTotal = float64(time.Now().UnixNano()-ts0) / proc.fuzzer.fuzzerConfig.MABTimeUnit
	}()
	// corpus := proc.fuzzer.corpusSnapshot()
	fuzzerSnapshot := proc.fuzzer.snapshot()
	ct := proc.fuzzer.choiceTable
	item := proc.fuzzer.workQueue.dequeueType(1, true, true)
	if item != nil { // If there're existing mutate work in the queue
		proc.fuzzer.writeLog("%s", "# WorkSmash\n")
		_, r := proc.ProcessItem(item)
		_r, ok := r.(ExecResult)
		if ok {
			ret.gainRaw = _r.gainRaw
			ret.calls = _r.calls
			ret.time = _r.time
			ret.pidx = _r.pidx
			ret.timeTotal = _r.timeTotal
		}
	} else {
		// MAB seed selection is integrated with chooseProgram
		pidx, _p := fuzzerSnapshot.chooseProgram(proc.rnd)
		K := len(fuzzerSnapshot.corpus)
		// pr := fuzzerSnapshot.corpusPrios
		if proc.fuzzer.fuzzerConfig.MABVerbose {
			proc.fuzzer.writeLog("- MAB Corpus Choice: %v/%v\n", pidx, K)
		}
		for i := 0; i < count; i++ {
			proc.fuzzer.writeLog("# %v Mutate %v\n", i, pidx)
			p := _p.Clone()
			p.ResetMAB()
			proc.fuzzer.logProgram(p)
			p.Mutate(proc.rnd, programLength, ct, fuzzerSnapshot.corpus)
			proc.fuzzer.logProgram(p)
			_, r := proc.execute(proc.execOpts, p, ProgNormal, StatFuzz)
			// proc.fuzzer.MABIncrementCorpusMutateCount(pidx, 1)
			ret.calls += r.calls
			ret.time += r.time
			ret.gainRaw += r.gainRaw
			ret.timeTotal += r.timeTotal
		}
		ret.pidx = pidx
	}
	return ret
}

func (proc *Proc) DoTriage() TriageResult {
	ret := TriageResult{
		minimizeGainRaw:  0.0,
		verifyGainRaw:    0.0,
		verifyTime:       0.0,
		verifyCalls:      0,
		minimizeTime:     0.0,
		minimizeCalls:    0,
		source:           -1,
		sourceCost:       0.0,
		minimizeTimeSave: 0.0,
		pidx:             -1,
		success:          false,
	}
	ts0 := time.Now().UnixNano()
	defer func() {
		ret.timeTotal = float64(time.Now().UnixNano()-ts0) / proc.fuzzer.fuzzerConfig.MABTimeUnit
	}()

	item := proc.fuzzer.workQueue.dequeueType(2, true, true)
	if item == nil {
		return ret
	}
	proc.fuzzer.writeLog("%s", "# WorkTriage\n")
	_, r := proc.ProcessItem(item)
	_ret, ok := r.(TriageResult)
	if !ok {
		return ret
	}
	ret = _ret
	proc.fuzzer.writeLog("# Triage Result: %+v\n", ret)
	return ret
}

func (proc *Proc) MABLoop() {
	// Triage first
	if proc.fuzzer.fuzzerConfig.MABTriageFirst {
		// Skip all MAB stuff for triaging
		item := proc.fuzzer.workQueue.dequeueType(2, true, true)
		if item != nil {
			proc.ProcessItem(item)
			return
		}
	}
	// Compute weight and proba
	ts0 := time.Now().UnixNano()
	weight := proc.fuzzer.MABGetWeight(true)
	// corpus := proc.fuzzer.corpusSnapshot()
	fuzzerSnapshot := proc.fuzzer.snapshot()
	triage_count := 1
	mutate_count := 1
	if len(fuzzerSnapshot.corpus) == 0 { // Check whether mutation is an option
		mutate_count = 0
	}
	proc.fuzzer.workQueue.mu.Lock() // Check whether triage is an option
	ql_triage := len(proc.fuzzer.workQueue.triage)
	ql_triageCandidate := len(proc.fuzzer.workQueue.triageCandidate)
	// proc.fuzzer.writeLog("- Triage Queue Length: %v + %v\n", ql_triage, ql_triageCandidate)
	proc.fuzzer.workQueue.mu.Unlock()
	if ql_triage+ql_triageCandidate == 0 {
		triage_count = 0
	}
	K := 1 + mutate_count + triage_count
	W := weight[0] + float64(mutate_count)*weight[1] + float64(triage_count)*weight[2]
	if W == 0.0 {
		log.Fatalf("WTF: Error total weight W = 0")
	}
	gamma := proc.fuzzer.MABGamma
	if proc.fuzzer.fuzzerConfig.MABAlgorithm == "Exp3-IX" {
		// No explicit exploration for Exp3-IX
		gamma = 0
	}
	pr_generate := (1-gamma)*weight[0]/W + gamma/float64(K)
	pr_mutate := (1-gamma)*weight[1]/W + gamma/float64(K)
	pr_triage := (1-gamma)*weight[2]/W + gamma/float64(K)
	if proc.fuzzer.fuzzerConfig.MABAlgorithm == "Exp3-IX" {
		pr_generate = weight[0] / W
		pr_mutate = weight[1] / W
		pr_triage = weight[2] / W
	}
	_pr_mutate := float64(mutate_count) * pr_mutate
	_pr_triage := float64(triage_count) * pr_triage
	pr_arr := []float64{pr_generate, _pr_mutate, _pr_triage} // Use real weight as pr. Consider cases where triage/mutation might be unavailable
	proc.fuzzer.writeLog("- MAB Probability: [%v, %v, %v]\n", pr_arr[0], pr_arr[1], pr_arr[2])
	// Choose
	rand_num := rand.Float64() * (pr_generate + _pr_mutate + _pr_triage)
	choice := -1
	if rand_num <= pr_generate {
		choice = 0
	} else if rand_num > pr_generate && rand_num <= pr_generate+_pr_mutate {
		choice = 1
	} else {
		choice = 2
	}
	ts1 := time.Now().UnixNano()
	proc.fuzzer.writeLog("- MAB Choose: %v\n", ts1-ts0)
	// Handle choices
	var r interface{}
	if choice == 0 {
		r = proc.DoGenerate()
	} else if choice == 1 {
		r = proc.DoMutate(proc.fuzzer.fuzzerConfig.mutateWeight)
	} else if choice == 2 {
		r = proc.DoTriage()
		// cost_before_min = float64(r.verifyTime) / 3.0 / proc.fuzzer.fuzzerConfig.MABTimeUnit
	}
	proc.fuzzer.writeLog("- MAB Choice: %v, Result: %+v\n", choice, r)
	// Update Weight
	proc.fuzzer.MABUpdateWeight(choice, r, pr_arr, K)
}

func (proc *Proc) loop() {
	generatePeriod := 100
	if proc.fuzzer.config.Flags&ipc.FlagSignal == 0 {
		// If we don't have real coverage signal, generate programs more frequently
		// because fallback signal is weak.
		generatePeriod = 2
	}
	for i := 0; ; i++ {
		// generatePeriod = proc.fuzzer.fuzzerConfig.mutateWeight/proc.fuzzer.fuzzerConfig.generateWeight + 1

		proc.fuzzer.MABRound += 1
		if proc.fuzzer.MABRound <= proc.fuzzer.fuzzerConfig.MABGenerateFirst {
			// Force Generate First
			ct := proc.fuzzer.choiceTable
			ts0 := time.Now().UnixNano()
			p := proc.fuzzer.target.Generate(proc.rnd, programLength, ct)
			proc.fuzzer.writeLog("# %v Generate\n", i)
			proc.fuzzer.logProgram(p)
			_, r := proc.execute(proc.execOpts, p, ProgNormal, StatGenerate)
			r.timeTotal = float64(time.Now().UnixNano()-ts0) / proc.fuzzer.fuzzerConfig.MABTimeUnit
			proc.fuzzer.writeLog("- Work Type: 0, Result: %+v\n", r)
			continue
		}
		if proc.fuzzer.fuzzerConfig.MABAlgorithm != "N/A" {
			proc.DoCandidate() // Deal with candidates first
			if proc.fuzzer.fuzzerConfig.MABDuration <= 0 || proc.fuzzer.MABRound < proc.fuzzer.fuzzerConfig.MABDuration {
				proc.MABLoop()
				continue
			} else if proc.fuzzer.fuzzerConfig.MABDuration > 0 && proc.fuzzer.MABRound >= proc.fuzzer.fuzzerConfig.MABDuration {
				// Reset params
				proc.fuzzer.ResetConfig()
			}
		}

		item := proc.fuzzer.workQueue.dequeue()
		if item != nil {
			itemType, r := proc.ProcessItem(item)
			if itemType == 1 && proc.fuzzer.fuzzerConfig.MABSeedSelection != "N/A" {
				proc.fuzzer.MABUpdateWeight(1, r, []float64{1.0, 1.0, 1.0}, 1.0)
			}
			proc.fuzzer.writeLog("- Work Type: %v, Result: %+v\n", itemType, r)
			// Don't count triage under NoMutations setup
			if itemType == 2 && proc.fuzzer.fuzzerConfig.MABNoMutations > 0 {
				proc.fuzzer.MABRound -= 1
			}
			continue
		}

		ct := proc.fuzzer.choiceTable
		fuzzerSnapshot := proc.fuzzer.snapshot()
		if len(fuzzerSnapshot.corpus) == 0 || i%generatePeriod == 0 || proc.fuzzer.MABRound < proc.fuzzer.fuzzerConfig.MABNoMutations {
			// Generate a new prog.
			ts0 := time.Now().UnixNano()
			p := proc.fuzzer.target.Generate(proc.rnd, programLength, ct)
			proc.fuzzer.writeLog("# %v Generate\n", i)
			proc.fuzzer.logProgram(p)
			_, r := proc.execute(proc.execOpts, p, ProgNormal, StatGenerate)
			r.timeTotal = float64(time.Now().UnixNano()-ts0) / proc.fuzzer.fuzzerConfig.MABTimeUnit
			proc.fuzzer.writeLog("- Work Type: 0, Result: %+v\n", r)
		} else {
			// Mutate an existing prog.
			ts0 := time.Now().UnixNano()
			pidx, p := fuzzerSnapshot.chooseProgram(proc.rnd)
			p = p.Clone()
			p.ResetMAB()
			proc.fuzzer.writeLog("# %v Mutate\n", i)
			proc.fuzzer.logProgram(p)
			p.Mutate(proc.rnd, programLength, ct, fuzzerSnapshot.corpus)
			proc.fuzzer.logProgram(p)
			_, r := proc.execute(proc.execOpts, p, ProgNormal, StatFuzz)
			r.pidx = pidx
			r.timeTotal = float64(time.Now().UnixNano()-ts0) / proc.fuzzer.fuzzerConfig.MABTimeUnit
			if proc.fuzzer.fuzzerConfig.MABSeedSelection != "N/A" {
				proc.fuzzer.MABUpdateWeight(1, r, []float64{1.0, 1.0, 1.0}, 1.0)
			}
			proc.fuzzer.writeLog("- Work Type: 1, Result: %+v\n", r)
		}
	}
}

func (proc *Proc) triageInput(item *WorkTriage) TriageResult {
	// We notify the manager as soon as the triage is scheduled.
	// Otherwise the triage might crash the VM and fuzzer will keep retrying
	ts_bgn := time.Now().UnixNano()
	source_data := item.p.Serialize()
	source_sig := hash.Hash(source_data)
	if proc.fuzzer.fuzzerConfig.syncTriage {
		proc.fuzzer.completeTriage(rpctype.RPCTriage{
			Sig:        source_sig,
			CallIndex:  item.call,
			Prog:       source_data,
			Flags:      int(item.flags),
			Info:       item.info,
			Source:     item.p.Source,
			SourceCost: item.p.MABCost,
		})
	}
	source := item.p.Source
	sourceCost := item.p.MABCost
	if _, ok := proc.fuzzer.MABTriageInfo[source_sig]; ok {
		source = proc.fuzzer.MABTriageInfo[source_sig].Source
		sourceCost = proc.fuzzer.MABTriageInfo[source_sig].SourceCost
	}
	ret := TriageResult{
		corpusGainRaw:    0.0,
		minimizeGainRaw:  0.0,
		verifyGainRaw:    0.0,
		verifyTime:       0.0,
		verifyCalls:      0,
		minimizeTime:     0.0,
		minimizeCalls:    0,
		source:           source,
		sourceSig:        source_sig,
		sourceCost:       sourceCost,
		minimizeTimeSave: 0.0,
		pidx:             -1,
		success:          false,
		timeTotal:        0.0,
	}
	size_bgn := len(item.p.Calls)
	// log.Logf(1, "#%v: triaging type=%x", proc.pid, item.flags)

	prio := signalPrio(item.p, &item.info, item.call)
	inputSignal := signal.FromRaw(item.info.Signal, prio)
	newSignal := proc.fuzzer.corpusSignalDiff(inputSignal)
	if newSignal.Empty() {
		return ret
	}
	callName := ".extra"
	logCallName := "extra"
	if item.call != -1 {
		callName = item.p.Calls[item.call].Meta.Name
		logCallName = fmt.Sprintf("call #%v %v", item.call, callName)
	}
	log.Logf(3, "triaging input for %v (new signal=%v)", logCallName, newSignal.Len())
	proc.fuzzer.writeLog("# Triaging NewSignal %v,%v,%v: %v^%v=%v\n", source_sig.String(), item.call, prio, inputSignal.Len(), proc.fuzzer.corpusSignal.Len(), newSignal.Len())

	var inputCover cover.Cover
	const (
		signalRuns       = 3
		minimizeAttempts = 3
	)
	// Compute input coverage and non-flaky signal for minimization.
	notexecuted := 0
	for i := 0; i < signalRuns; i++ {
		info, _time := proc.executeRaw(proc.execOptsCover, item.p, StatTriage)
		ret.verifyTime += _time
		if !reexecutionSuccess(info, &item.info, item.call) {
			// The call was not executed or failed.
			notexecuted++
			if notexecuted > signalRuns/2+1 {
				ret.timeTotal = float64(time.Now().UnixNano()-ts_bgn) / proc.fuzzer.fuzzerConfig.MABTimeUnit
				ret.verifyCalls = (i + 1) * size_bgn
				ret.corpusGainRaw = 0.0
				return ret // if happens too often, give up
			}
			continue
		}
		thisSignal, thisCover := getSignalAndCover(item.p, info, item.call)
		newSignal = newSignal.Intersection(thisSignal)
		// Without !minimized check manager starts losing some considerable amount
		// of coverage after each restart. Mechanics of this are not completely clear.
		if newSignal.Empty() && item.flags&ProgMinimized == 0 {
			ret.timeTotal = float64(time.Now().UnixNano()-ts_bgn) / proc.fuzzer.fuzzerConfig.MABTimeUnit
			ret.verifyCalls = (i + 1) * size_bgn
			ret.verifyGainRaw = 0.0
			return ret
		}
		inputCover.Merge(thisCover)
	}
	ret.success = true
	if ret.sourceCost == 0.0 {
		ret.sourceCost = ret.verifyTime / 3.0
	} else {
		ret.sourceCost = (ret.verifyTime + ret.sourceCost) / 4.0
	}
	proc.fuzzer.writeLog("# %v Minimize\n", 0)
	proc.fuzzer.logProgram(item.p)
	minimizeCalls := 0
	minimizeGainRaw := 0.0
	minimizeTimeAfter := sourceCost
	if item.flags&ProgMinimized == 0 {
		item.p, item.call = prog.Minimize(item.p, item.call, false,
			func(p1 *prog.Prog, call1 int) bool {
				proc.fuzzer.writeLog("%s\n", "# Minimize Attempt")
				p1.Source = 2
				proc.fuzzer.logProgram(p1)
				t_avg := 0.0

				for i := 0; i < minimizeAttempts; i++ {
					var info *ipc.ProgInfo
					var r ExecResult
					info, r = proc.execute(proc.execOptsNoCollide, p1, ProgNormal, StatMinimize)
					minimizeGainRaw += r.gainRaw
					ret.minimizeTime += r.time
					t_avg += r.time
					minimizeCalls += len(p1.Calls)

					if !reexecutionSuccess(info, &item.info, call1) {
						// The call was not executed or failed.
						continue
					}
					thisSignal, _ := getSignalAndCover(p1, info, call1)
					if newSignal.Intersection(thisSignal).Len() == newSignal.Len() {
						proc.fuzzer.writeLog("%s\n", "# Minimize Success")
						t_avg = t_avg / float64(i+1)
						minimizeTimeAfter = t_avg

						return true
					}
				}
				return false
			})
	}

	proc.fuzzer.writeLog("%s\n", "# Minimize Final")
	proc.fuzzer.logProgram(item.p)

	data := item.p.Serialize()
	sig := hash.Hash(data)

	corpusGainRaw := 0.0
	if len(inputSignal) > 0 {
		corpusGainRaw = float64(len(newSignal))
	}

	log.Logf(2, "added new input for %v to corpus:\n%s", logCallName, data)
	proc.fuzzer.sendInputToManager(rpctype.RPCInput{
		Call:   callName,
		Prog:   data,
		Signal: inputSignal.Serialize(),
		Cover:  inputCover.Serialize(),
		CorpusGLC: rpctype.CorpusGLC{
			VerifyGain:       0.0,
			VerifyCost:       ret.verifyTime,
			MinimizeGain:     minimizeGainRaw,
			MinimizeCost:     ret.minimizeTime,
			MinimizeTimeSave: sourceCost - minimizeTimeAfter,
		},
	})

	proc.fuzzer.writeLog("# addInputToCorpus %x: %v. Source: %v\n", sig, proc.fuzzer.corpusSignal.Len(), ret.source)
	proc.fuzzer.logSignal(inputSignal, "+")
	ret.pidx = proc.fuzzer.addInputToCorpus(item.p, inputSignal, sig)

	if item.flags&ProgSmashed == 0 {
		numSmashes := int(math.Floor(float64(proc.fuzzer.fuzzerConfig.smashWeight) / float64(proc.fuzzer.fuzzerConfig.mutateWeight)))
		// if numSmashes == 0 {
		//      numSmashes = 1
		// }
		for i := 0; i < numSmashes; i++ {
			proc.fuzzer.workQueue.enqueue(&WorkSmash{item.p, item.call, proc.fuzzer.fuzzerConfig.mutateWeight})
		}

	}

	ts_end := time.Now().UnixNano()
	ret.timeTotal = float64(ts_end-ts_bgn) / proc.fuzzer.fuzzerConfig.MABTimeUnit
	ret.minimizeGainRaw = minimizeGainRaw
	ret.corpusGainRaw = corpusGainRaw
	ret.minimizeTimeSave = sourceCost - minimizeTimeAfter
	// ret.minimizeTime = t_minimize
	// ret.verifyTime = t_corpus
	ret.minimizeCalls = minimizeCalls
	ret.verifyCalls = size_bgn * signalRuns
	ret.success = true
	return ret
}

func reexecutionSuccess(info *ipc.ProgInfo, oldInfo *ipc.CallInfo, call int) bool {
	if info == nil || len(info.Calls) == 0 {
		return false
	}
	if call != -1 {
		// Don't minimize calls from successful to unsuccessful.
		// Successful calls are much more valuable.
		if oldInfo.Errno == 0 && info.Calls[call].Errno != 0 {
			return false
		}
		return len(info.Calls[call].Signal) != 0
	}
	return len(info.Extra.Signal) != 0
}

func getSignalAndCover(p *prog.Prog, info *ipc.ProgInfo, call int) (signal.Signal, []uint32) {
	inf := &info.Extra
	if call != -1 {
		inf = &info.Calls[call]
	}
	return signal.FromRaw(inf.Signal, signalPrio(p, inf, call)), inf.Cover
}

func (proc *Proc) smashInput(item *WorkSmash) ExecResult {
	ret := ExecResult{
		gainRaw:   0.0,
		time:      0.0,
		calls:     0,
		pidx:      -1,
		count:     0,
		timeTotal: 0.0,
	}
	ts_bgn := time.Now().UnixNano()
	sig := hash.Hash(item.p.Serialize())

	if proc.fuzzer.faultInjectionEnabled && item.call != -1 {
		proc.fuzzer.writeLog("- failCall: %v, %v\n", sig.String, item.call)
		proc.failCall(item.p, item.call)
	}
	if proc.fuzzer.comparisonTracingEnabled && item.call != -1 {
		proc.fuzzer.writeLog("- executeHintSeed: %v, %v\n", sig.String, item.call)
		proc.executeHintSeed(item.p, item.call)
	}
	fuzzerSnapshot := proc.fuzzer.snapshot()
	pidx := -1
	if v, ok := proc.fuzzer.corpusHashes[sig]; ok {
		pidx = v
		ret.pidx = pidx
	}
	// Mark for update if we're syncing smash
	if pidx > 0 && proc.fuzzer.fuzzerConfig.syncSmash {
		proc.fuzzer.MABCorpusUpdate[pidx] = 1
		item.p.Smashed = true
		proc.fuzzer.smashesFinished = append(proc.fuzzer.smashesFinished, sig)
	}

	for i := 0; i < item.count; i++ {
		p := item.p.Clone()
		p.ResetMAB()
		proc.fuzzer.writeLog("# %v Mutate Smash\n", i)
		proc.fuzzer.logProgram(p)

		p.Mutate(proc.rnd, programLength, proc.fuzzer.choiceTable, fuzzerSnapshot.corpus)

		proc.fuzzer.logProgram(p)
		_, r := proc.execute(proc.execOpts, p, ProgNormal, StatSmash)
		if proc.fuzzer.fuzzerConfig.MABAlgorithm == "N/A" && proc.fuzzer.fuzzerConfig.MABSeedSelection == "N/A" && proc.fuzzer.fuzzerConfig.syncSmash {
			// Without MAB, we need to count mutations for smash syncing
			proc.fuzzer.MABIncrementCorpusMutateCount(pidx, 1)
		}
		ret.count += 1
		ret.calls += r.calls
		ret.gainRaw += r.gainRaw
		ret.time += r.time
	}
	ts_end := time.Now().UnixNano()
	ret.timeTotal = float64(ts_end-ts_bgn) / proc.fuzzer.fuzzerConfig.MABTimeUnit
	return ret

}

func (proc *Proc) failCall(p *prog.Prog, call int) {
	for nth := 0; nth < 100; nth++ {
		log.Logf(1, "#%v: injecting fault into call %v/%v", proc.pid, call, nth)
		opts := *proc.execOpts
		opts.Flags |= ipc.FlagInjectFault
		opts.FaultCall = call
		opts.FaultNth = nth
		info, _ := proc.executeRaw(&opts, p, StatSmash)
		if info != nil && len(info.Calls) > call && info.Calls[call].Flags&ipc.CallFaultInjected == 0 {
			break
		}
	}
}

func (proc *Proc) executeHintSeed(p *prog.Prog, call int) {
	log.Logf(1, "#%v: collecting comparisons", proc.pid)
	// First execute the original program to dump comparisons from KCOV.
	info, _ := proc.execute(proc.execOptsComps, p, ProgNormal, StatSeed)
	if info == nil {
		return
	}

	// Then mutate the initial program for every match between
	// a syscall argument and a comparison operand.
	// Execute each of such mutants to check if it gives new coverage.
	p.MutateWithHints(call, info.Calls[call].Comps, func(p *prog.Prog) {
		log.Logf(1, "#%v: executing comparison hint", proc.pid)
		proc.execute(proc.execOpts, p, ProgNormal, StatHint)
	})
}

func (proc *Proc) execute(execOpts *ipc.ExecOpts, p *prog.Prog, flags ProgTypes, stat Stat) (*ipc.ProgInfo, ExecResult) {
	ret := ExecResult{
		gainRaw: 0.0,
		time:    0,
		calls:   0,
		pidx:    -1,
	}
	ts_bgn := time.Now().UnixNano()
	info, _time := proc.executeRaw(execOpts, p, stat)
	ret.time += _time
	p.MABCost = _time
	calls, extra, gainRaw := proc.fuzzer.checkNewSignal(p, info)
	ts_end := time.Now().UnixNano()
	for _, callIndex := range calls {
		proc.enqueueCallTriage(p, flags, callIndex, info.Calls[callIndex])
	}
	if extra {
		proc.enqueueCallTriage(p, flags, -1, info.Extra)
	}
	ret.calls = ((proc.fuzzer.fuzzerConfig.executeRetries + 1) * len(p.Calls))
	ret.timeTotal = float64(ts_end-ts_bgn) / proc.fuzzer.fuzzerConfig.MABTimeUnit
	ret.gainRaw = gainRaw
	return info, ret
}

func (proc *Proc) enqueueCallTriage(p *prog.Prog, flags ProgTypes, callIndex int, info ipc.CallInfo) {
	// info.Signal points to the output shmem region, detach it before queueing.
	info.Signal = append([]uint32{}, info.Signal...)
	// None of the caller use Cover, so just nil it instead of detaching.
	// Note: triage input uses executeRaw to get coverage.
	info.Cover = nil
	proc.fuzzer.workQueue.enqueue(&WorkTriage{
		p:     p.Clone(),
		call:  callIndex,
		info:  info,
		flags: flags,
	})
	if proc.fuzzer.fuzzerConfig.syncTriage {
		data := p.Serialize()
		sig := hash.Hash(data)
		proc.fuzzer.newTriage(rpctype.RPCTriage{
			Sig:        sig,
			CallIndex:  callIndex,
			Prog:       data,
			Flags:      int(flags),
			Info:       info,
			Source:     p.Source,
			SourceCost: p.MABCost,
		})
	}
}

func (proc *Proc) executeRaw(opts *ipc.ExecOpts, p *prog.Prog, stat Stat) (*ipc.ProgInfo, float64) {
	if opts.Flags&ipc.FlagDedupCover == 0 {
		log.Fatalf("dedup cover is not enabled")
	}
	ts := time.Now().UnixNano()

	// Limit concurrency window and do leak checking once in a while.
	ticket := proc.fuzzer.gate.Enter()
	defer proc.fuzzer.gate.Leave(ticket)

	proc.logProgram(opts, p)
	for try := 0; ; try++ {
		atomic.AddUint64(&proc.fuzzer.stats[stat], 1)
		output, info, hanged, err := proc.env.Exec(opts, p)
		if err != nil {
			if try > 10 {
				log.Fatalf("executor %v failed %v times:\n%v", proc.pid, try, err)
			}
			log.Logf(4, "fuzzer detected executor failure='%v', retrying #%d", err, try+1)
			debug.FreeOSMemory()
			time.Sleep(time.Second)
			continue
		}
		log.Logf(2, "result hanged=%v: %s", hanged, output)
		ts_end := time.Now().UnixNano()
		return info, float64(ts_end-ts) / proc.fuzzer.fuzzerConfig.MABTimeUnit
	}
}

func (proc *Proc) logProgram(opts *ipc.ExecOpts, p *prog.Prog) {
	if proc.fuzzer.outputType == OutputNone {
		return
	}

	data := p.Serialize()
	strOpts := ""
	if opts.Flags&ipc.FlagInjectFault != 0 {
		strOpts = fmt.Sprintf(" (fault-call:%v fault-nth:%v)", opts.FaultCall, opts.FaultNth)
	}

	// The following output helps to understand what program crashed kernel.
	// It must not be intermixed.
	switch proc.fuzzer.outputType {
	case OutputStdout:
		now := time.Now()
		proc.fuzzer.logMu.Lock()
		fmt.Printf("%02v:%02v:%02v executing program %v%v:\n%s\n",
			now.Hour(), now.Minute(), now.Second(),
			proc.pid, strOpts, data)
		proc.fuzzer.logMu.Unlock()
	case OutputDmesg:
		fd, err := syscall.Open("/dev/kmsg", syscall.O_WRONLY, 0)
		if err == nil {
			buf := new(bytes.Buffer)
			fmt.Fprintf(buf, "syzkaller: executing program %v%v:\n%s\n",
				proc.pid, strOpts, data)
			syscall.Write(fd, buf.Bytes())
			syscall.Close(fd)
		}
	case OutputFile:
		f, err := os.Create(fmt.Sprintf("%v-%v.prog", proc.fuzzer.name, proc.pid))
		if err == nil {
			if strOpts != "" {
				fmt.Fprintf(f, "#%v\n", strOpts)
			}
			f.Write(data)
			f.Close()
		}
	default:
		log.Fatalf("unknown output type: %v", proc.fuzzer.outputType)
	}
}
