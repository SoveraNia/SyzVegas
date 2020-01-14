// Copyright 2015 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// MODIFIED: Daimeng Wang

package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/syzkaller/pkg/csource"
	"github.com/google/syzkaller/pkg/glc"
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/host"
	"github.com/google/syzkaller/pkg/ipc"
	"github.com/google/syzkaller/pkg/ipc/ipcconfig"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/osutil"
	"github.com/google/syzkaller/pkg/rpctype"
	"github.com/google/syzkaller/pkg/signal"
	"github.com/google/syzkaller/prog"
	_ "github.com/google/syzkaller/sys"
)

type Fuzzer struct {
	name              string
	outputType        OutputType
	config            *ipc.Config
	execOpts          *ipc.ExecOpts
	procs             []*Proc
	gate              *ipc.Gate
	workQueue         *WorkQueue
	needPoll          chan struct{}
	choiceTable       *prog.ChoiceTable
	stats             [StatCount]uint64
	manager           *rpctype.RPCClient
	target            *prog.Target
	triagedCandidates uint32

	feedback     string
	fuzzerConfig FuzzerConfig

	faultInjectionEnabled    bool
	comparisonTracingEnabled bool

	corpusMu       sync.RWMutex
	corpus         []*prog.Prog
	corpusHashes   map[hash.Sig]int
	corpusPrios    []float64
	corpusPriosSum []float64
	sumPrios       float64

	signalMu     sync.RWMutex
	corpusSignal signal.Signal // signal of inputs in corpus
	maxSignal    signal.Signal // max signal ever observed including flakes
	newSignal    signal.Signal // diff of maxSignal since last sync with master

	logMu sync.Mutex

	// MAB stuff
	triages           map[hash.Sig]int                 // All triage signatures, 0->Unfin. 1->Fin
	triagesUnfinished map[hash.Sig][]rpctype.RPCTriage // Buffer for unfinished triages to send to manager
	smashesFinished   []hash.Sig

	loggedPrograms map[hash.Sig]int

	MABMu    sync.RWMutex
	MABGamma float64 // No reset
	MABEta   float64 // No reset

	MABCorpusGamma float64
	MABCorpusEta   float64

	MABRound          int        // How many MAB choices have been made. No reset
	MABExp31Round     int        // How many rounds of Exp3.1. No reset
	MABExp31Threshold float64    // Threshold based on Round. No sync
	MABGLC            glc.MABGLC // {Generate, Mutate, Triage}. Used for stationary bandit

	MABCorpusUpdate map[int]int
	MABTriageInfo   map[hash.Sig]*glc.TriageInfo

	MABGMTStatus
}

type FuzzerSnapshot struct {
	fuzzerConfig   *FuzzerConfig
	corpus         []*prog.Prog
	corpusPrios    []float64
	corpusPriosSum []float64
	sumPrios       float64
	workQueue      *WorkQueue
}

type FuzzerConfig struct {
	executeRetries     int
	signalRunThreshold float64
	noMinimization     bool
	generateWeight     int
	mutateWeight       int
	smashWeight        int
	syncTriage         bool
	syncSmash          bool
	verifyFirst        bool

	MABAlgorithm     string
	MABSeedSelection string
	MABTargetCorpus  bool
	MABVerbose       bool
	ProgVerbose      bool
	MABTimeUnit      float64
	MABTriageFirst   bool
	MABZLogNormalize bool
	MABNormalize     int
	MABExp31         bool
	MABDuration      int
	MABGenerateFirst int
	MABNoMutations   int
}

func (fuzzer *Fuzzer) ResetConfig() {
	fuzzer.fuzzerConfig.executeRetries = 0
	fuzzer.fuzzerConfig.signalRunThreshold = 0.0
	fuzzer.fuzzerConfig.noMinimization = false
	fuzzer.fuzzerConfig.generateWeight = 1
	fuzzer.fuzzerConfig.mutateWeight = 100
	fuzzer.fuzzerConfig.smashWeight = 100
	fuzzer.fuzzerConfig.syncTriage = false
	fuzzer.fuzzerConfig.syncSmash = false
	fuzzer.fuzzerConfig.verifyFirst = false
	fuzzer.fuzzerConfig.MABAlgorithm = "N/A"
	fuzzer.fuzzerConfig.MABSeedSelection = "N/A"
	fuzzer.fuzzerConfig.MABTargetCorpus = false
	fuzzer.fuzzerConfig.MABVerbose = false
	fuzzer.fuzzerConfig.MABTimeUnit = 1000000.0
	fuzzer.fuzzerConfig.MABTriageFirst = false
	fuzzer.fuzzerConfig.MABZLogNormalize = false
	fuzzer.fuzzerConfig.MABNormalize = -1
	fuzzer.fuzzerConfig.MABExp31 = false
	fuzzer.fuzzerConfig.MABDuration = 0
	fuzzer.fuzzerConfig.MABGenerateFirst = 0
	fuzzer.fuzzerConfig.MABNoMutations = 0
}

func (fuzzer *Fuzzer) printFuzzerConfig(config FuzzerConfig) {
	fmt.Printf("# Fuzzer Config:\n%+v\n", config)
	fmt.Printf("# MABEta = %v, MABGamma = %v\n", fuzzer.MABEta, fuzzer.MABGamma)
}

type Stat int

const (
	StatGenerate Stat = iota
	StatFuzz
	StatCandidate
	StatTriage
	StatMinimize
	StatSmash
	StatHint
	StatSeed
	StatCount
)

var statNames = [StatCount]string{
	StatGenerate:  "exec gen",
	StatFuzz:      "exec fuzz",
	StatCandidate: "exec candidate",
	StatTriage:    "exec triage",
	StatMinimize:  "exec minimize",
	StatSmash:     "exec smash",
	StatHint:      "exec hints",
	StatSeed:      "exec seeds",
}

type OutputType int

const (
	OutputNone OutputType = iota
	OutputStdout
	OutputDmesg
	OutputFile
)

func main() {
	debug.SetGCPercent(50)

	var (
		flagName    = flag.String("name", "test", "unique name for manager")
		flagOS      = flag.String("os", runtime.GOOS, "target OS")
		flagArch    = flag.String("arch", runtime.GOARCH, "target arch")
		flagManager = flag.String("manager", "", "manager rpc address")
		flagProcs   = flag.Int("procs", 1, "number of parallel test processes")
		flagOutput  = flag.String("output", "stdout", "write programs to none/stdout/dmesg/file")
		flagPprof   = flag.String("pprof", "", "address to serve pprof profiles")
		flagTest    = flag.Bool("test", false, "enable image testing mode")      // used by syz-ci
		flagRunTest = flag.Bool("runtest", false, "enable program testing mode") // used by pkg/runtest

		// MOD
		flagFeedback                       = flag.String("feedback", "KCOV", "Source of feedback")
		flagFuzzerConfigExecuteRetries     = flag.Int("fuzzerconfig_executeRetries", 0, "Number of extra executeRaw() during execute()")
		flagFuzzerConfigSignalRunThreshold = flag.Float64("fuzzerconfig_signalRunThreshold", 0.0, "Threshold for signalRuns during triaging")
		flagFuzzerConfigNoMinimization     = flag.Bool("fuzzerconfig_noMinimization", false, "Do not do minimization during triaging")
		flagFuzzerConfigGenerateWeight     = flag.Int("fuzzerconfig_generateWeight", 1, "Mutation-to-Generation Weight")
		flagFuzzerConfigMutateWeight       = flag.Int("fuzzerconfig_mutateWeight", 100, "Mutation-to-Generation Weight")
		flagFuzzerConfigSmashWeight        = flag.Int("fuzzerconfig_smashWeight", 100, "Smash-to-Generation Weight")
		flagFuzzerConfigSyncTriage         = flag.Bool("fuzzerconfig_syncTriage", false, "Sync triage with manager")
		flagFuzzerConfigSyncSmash          = flag.Bool("fuzzerconfig_syncSmash", false, "Sync smash with manager")
		flagFuzzerConfigVerifyFirst        = flag.Bool("fuzzerconfig_verifyFirst", false, "Verify signal during gen/mut instead of tri")
		flagFuzzerConfigMABAlgorithm       = flag.String("fuzzerconfig_MABAlgorithm", "N/A", "Which algorithm to use for multi-armed-bandit: Exp3-Gain/Exl3-Loss/Exp3-IX")
		flagFuzzerConfigMABTargetCorpus    = flag.Bool("fuzzerconfig_MABTargetCorpus", false, "Let MAB target corpus signal")
		flagFuzzerConfigMABSeedSelection   = flag.String("fuzzerconfig_MABSeedSelection", "N/A", "Use MAB for seed selection")
		flagFuzzerConfigMABVerbose         = flag.Bool("fuzzerconfig_MABVerbose", false, "Verbose MAB-related info")
		flagFuzzerConfigProgVerbose        = flag.Bool("fuzzerconfig_ProgVerbose", false, "Verbose Program content")
		flagFuzzerConfigMABZLogNormalize   = flag.Bool("fuzzerconfig_MABZLogNormalize", false, "Use Z-Log for normalization")
		flagFuzzerConfigMABTriageFirst     = flag.Bool("fuzzerconfig_MABTriageFirst", false, "Triage first for MAB scheduling")
		flagFuzzerConfigMABNormalize       = flag.Int("fuzzerconfig_MABNormalize", -1, "Normalize the gain and losses. <0: No normalize, =0: Max-min normalize, >0 Window normalize")
		flagFuzzerConfigMABTimeUnit        = flag.Float64("fuzzerconfig_MABTimeUnit", 0.0, "Use time average. <=0 for disable")
		flagFuzzerConfigMABExp31           = flag.Bool("fuzzerconfig_MABExp31", false, "Use exp3.1 algorithm to reset MAB periodically")
		flagFuzzerConfigMABDuration        = flag.Int("fuzzerconfig_MABDuration", -1, "# Rounds of MAB. <=0 to disable")
		flagFuzzerConfigMABGenerateFirst   = flag.Int("fuzzerconfig_MABGenerateFirst", -1, "Generate X programs before doing anything else. <=0 to disable")
		flagFuzzerConfigMABNoMutations     = flag.Int("fuzzerconfig_MABNoMutations", -1, "Don't mutate at all before K generations. <=0 to disable")
		flagFuzzerConfigMABGamma           = flag.Float64("fuzzerconfig_MABGamma", 0.1, "Exploration factor")
		flagFuzzerConfigMABEta             = flag.Float64("fuzzerconfig_MABEta", 0.1, "Weight increase factor")
		flagFuzzerConfigMABCorpusGamma     = flag.Float64("fuzzerconfig_MABCorpusGamma", 0.05, "Exploration factor")
		flagFuzzerConfigMABCorpusEta       = flag.Float64("fuzzerconfig_MABCorpusEta", 0.1, "Weight increase factor")
	)
	flag.Parse()
	outputType := parseOutputType(*flagOutput)
	log.Logf(0, "fuzzer started")

	fuzzerConfig := FuzzerConfig{
		executeRetries:     *flagFuzzerConfigExecuteRetries,
		signalRunThreshold: *flagFuzzerConfigSignalRunThreshold,
		noMinimization:     *flagFuzzerConfigNoMinimization,
		generateWeight:     *flagFuzzerConfigGenerateWeight,
		mutateWeight:       *flagFuzzerConfigMutateWeight,
		smashWeight:        *flagFuzzerConfigSmashWeight,
		syncTriage:         *flagFuzzerConfigSyncTriage,
		syncSmash:          *flagFuzzerConfigSyncSmash,
		verifyFirst:        *flagFuzzerConfigVerifyFirst,
		MABAlgorithm:       *flagFuzzerConfigMABAlgorithm,
		MABTargetCorpus:    *flagFuzzerConfigMABTargetCorpus,
		MABSeedSelection:   *flagFuzzerConfigMABSeedSelection,
		MABVerbose:         *flagFuzzerConfigMABVerbose,
		ProgVerbose:        *flagFuzzerConfigProgVerbose,
		MABTimeUnit:        *flagFuzzerConfigMABTimeUnit,
		MABTriageFirst:     *flagFuzzerConfigMABTriageFirst,
		MABZLogNormalize:   *flagFuzzerConfigMABZLogNormalize,
		MABNormalize:       *flagFuzzerConfigMABNormalize,
		MABExp31:           *flagFuzzerConfigMABExp31,
		MABDuration:        *flagFuzzerConfigMABDuration,
		MABGenerateFirst:   *flagFuzzerConfigMABGenerateFirst,
		MABNoMutations:     *flagFuzzerConfigMABNoMutations,
	}
	if fuzzerConfig.MABTimeUnit == 0.0 {
		fuzzerConfig.MABTimeUnit = 1000000.0
	}

	target, err := prog.GetTarget(*flagOS, *flagArch)
	if err != nil {
		log.Fatalf("%v", err)
	}

	config, execOpts, err := ipcconfig.Default(target)
	if err != nil {
		log.Fatalf("failed to create default ipc config: %v", err)
	}
	sandbox := ipc.FlagsToSandbox(config.Flags)
	shutdown := make(chan struct{})
	osutil.HandleInterrupts(shutdown)
	go func() {
		// Handles graceful preemption on GCE.
		<-shutdown
		log.Logf(0, "SYZ-FUZZER: PREEMPTED")
		os.Exit(1)
	}()

	checkArgs := &checkArgs{
		target:      target,
		sandbox:     sandbox,
		ipcConfig:   config,
		ipcExecOpts: execOpts,
	}
	if *flagTest {
		testImage(*flagManager, checkArgs)
		return
	}

	if *flagPprof != "" {
		go func() {
			err := http.ListenAndServe(*flagPprof, nil)
			log.Fatalf("failed to serve pprof profiles: %v", err)
		}()
	} else {
		runtime.MemProfileRate = 0
	}

	log.Logf(0, "dialing manager at %v", *flagManager)
	manager, err := rpctype.NewRPCClient(*flagManager)
	if err != nil {
		log.Fatalf("failed to connect to manager: %v ", err)
	}
	a := &rpctype.ConnectArgs{Name: *flagName}
	r := &rpctype.ConnectRes{}
	if err := manager.Call("Manager.Connect", a, r); err != nil {
		log.Fatalf("failed to connect to manager: %v ", err)
	}
	featureFlags, err := csource.ParseFeaturesFlags("none", "none", true)
	if err != nil {
		log.Fatal(err)
	}
	if r.CheckResult == nil {
		checkArgs.gitRevision = r.GitRevision
		checkArgs.targetRevision = r.TargetRevision
		checkArgs.enabledCalls = r.EnabledCalls
		checkArgs.allSandboxes = r.AllSandboxes
		checkArgs.featureFlags = featureFlags
		r.CheckResult, err = checkMachine(checkArgs)
		if err != nil {
			if r.CheckResult == nil {
				r.CheckResult = new(rpctype.CheckArgs)
			}
			r.CheckResult.Error = err.Error()
		}
		r.CheckResult.Name = *flagName
		if err := manager.Call("Manager.Check", r.CheckResult, nil); err != nil {
			log.Fatalf("Manager.Check call failed: %v", err)
		}
		if r.CheckResult.Error != "" {
			log.Fatalf("%v", r.CheckResult.Error)
		}
	} else {
		if err = host.Setup(target, r.CheckResult.Features, featureFlags, config.Executor); err != nil {
			log.Fatal(err)
		}
	}
	log.Logf(0, "syscalls: %v", len(r.CheckResult.EnabledCalls[sandbox]))
	for _, feat := range r.CheckResult.Features.Supported() {
		log.Logf(0, "%v: %v", feat.Name, feat.Reason)
	}
	if r.CheckResult.Features[host.FeatureExtraCoverage].Enabled {
		config.Flags |= ipc.FlagExtraCover
	}
	if r.CheckResult.Features[host.FeatureNetInjection].Enabled {
		config.Flags |= ipc.FlagEnableTun
	}
	if r.CheckResult.Features[host.FeatureNetDevices].Enabled {
		config.Flags |= ipc.FlagEnableNetDev
	}
	config.Flags |= ipc.FlagEnableNetReset
	config.Flags |= ipc.FlagEnableCgroups
	config.Flags |= ipc.FlagEnableCloseFds
	if r.CheckResult.Features[host.FeatureDevlinkPCI].Enabled {
		config.Flags |= ipc.FlagEnableDevlinkPCI
	}

	if *flagRunTest {
		runTest(target, manager, *flagName, config.Executor)
		return
	}

	needPoll := make(chan struct{}, 1)
	needPoll <- struct{}{}
	fuzzer := &Fuzzer{
		name:                     *flagName,
		outputType:               outputType,
		config:                   config,
		execOpts:                 execOpts,
		workQueue:                newWorkQueue(*flagProcs, needPoll),
		needPoll:                 needPoll,
		manager:                  manager,
		target:                   target,
		faultInjectionEnabled:    r.CheckResult.Features[host.FeatureFault].Enabled,
		comparisonTracingEnabled: r.CheckResult.Features[host.FeatureComparisons].Enabled,

		// MOD
		corpusHashes:      make(map[hash.Sig]int),
		MABCorpusUpdate:   make(map[int]int),
		triages:           make(map[hash.Sig]int),
		triagesUnfinished: make(map[hash.Sig][]rpctype.RPCTriage),
		loggedPrograms:    make(map[hash.Sig]int),
		feedback:          *flagFeedback,
		fuzzerConfig:      fuzzerConfig,
		MABGLC:            glc.MABGLC{},
		MABGamma:          *flagFuzzerConfigMABGamma,
		MABEta:            *flagFuzzerConfigMABEta,
		MABCorpusGamma:    *flagFuzzerConfigMABCorpusGamma,
		MABCorpusEta:      *flagFuzzerConfigMABCorpusEta,
		MABRound:          0,
		MABExp31Round:     1,
		MABTriageInfo:     make(map[hash.Sig]*glc.TriageInfo),
	}
	fuzzer.workQueue.fuzzer = fuzzer
	// Initialize params for Exp31
	if fuzzer.fuzzerConfig.MABExp31 {
		fuzzer.MABBootstrapExp31()
	}
	fmt.Printf("Fuzzer Feedback: %s\n", fuzzer.feedback)
	fuzzer.printFuzzerConfig(fuzzer.fuzzerConfig)

	gateCallback := fuzzer.useBugFrames(r, *flagProcs)
	fuzzer.gate = ipc.NewGate(2**flagProcs, gateCallback)

	for i := 0; fuzzer.poll(i == 0, i == 0, nil); i++ {
	}
	calls := make(map[*prog.Syscall]bool)
	for _, id := range r.CheckResult.EnabledCalls[sandbox] {
		calls[target.Syscalls[id]] = true
	}
	prios := target.CalculatePriorities(fuzzer.corpus)
	fuzzer.choiceTable = target.BuildChoiceTable(prios, calls)

	for pid := 0; pid < *flagProcs; pid++ {
		proc, err := newProc(fuzzer, pid)
		if err != nil {
			log.Fatalf("failed to create proc: %v", err)
		}
		fuzzer.procs = append(fuzzer.procs, proc)
		go proc.loop()
	}

	fuzzer.pollLoop()
}

func (fuzzer *Fuzzer) writeLog(format string, args ...interface{}) {
	fuzzer.logMu.Lock()
	fmt.Printf(format, args...)
	fuzzer.logMu.Unlock()
	return
}

func (fuzzer *Fuzzer) logProgram(p *prog.Prog) {
	data := p.Serialize()
	sig := hash.Hash(data)
	if fuzzer.fuzzerConfig.ProgVerbose {
		if _, ok := fuzzer.loggedPrograms[sig]; !ok {
			s := strings.ReplaceAll(string(data), "\n", "\n> ")
			fuzzer.writeLog(">>> %s\n> %s\n<<<\n", sig.String(), s)
			fuzzer.loggedPrograms[sig] = 1
		} else {
			fuzzer.writeLog(">>> %s\n<<<\n", sig.String())
		}
	} else {
		fuzzer.writeLog(">>> %s\n<<<\n", sig.String())
	}
}

// Returns gateCallback for leak checking if enabled.
func (fuzzer *Fuzzer) useBugFrames(r *rpctype.ConnectRes, flagProcs int) func() {
	var gateCallback func()

	if r.CheckResult.Features[host.FeatureLeak].Enabled {
		gateCallback = func() { fuzzer.gateCallback(r.MemoryLeakFrames) }
	}

	if r.CheckResult.Features[host.FeatureKCSAN].Enabled && len(r.DataRaceFrames) != 0 {
		fuzzer.blacklistDataRaceFrames(r.DataRaceFrames)
	}

	return gateCallback
}

func (fuzzer *Fuzzer) gateCallback(leakFrames []string) {
	// Leak checking is very slow so we don't do it while triaging the corpus
	// (otherwise it takes infinity). When we have presumably triaged the corpus
	// (triagedCandidates == 1), we run leak checking bug ignore the result
	// to flush any previous leaks. After that (triagedCandidates == 2)
	// we do actual leak checking and report leaks.
	triagedCandidates := atomic.LoadUint32(&fuzzer.triagedCandidates)
	if triagedCandidates == 0 {
		return
	}
	args := append([]string{"leak"}, leakFrames...)
	output, err := osutil.RunCmd(10*time.Minute, "", fuzzer.config.Executor, args...)
	if err != nil && triagedCandidates == 2 {
		// If we exit right away, dying executors will dump lots of garbage to console.
		os.Stdout.Write(output)
		fmt.Printf("BUG: leak checking failed")
		time.Sleep(time.Hour)
		os.Exit(1)
	}
	if triagedCandidates == 1 {
		atomic.StoreUint32(&fuzzer.triagedCandidates, 2)
	}
}

func (fuzzer *Fuzzer) blacklistDataRaceFrames(frames []string) {
	args := append([]string{"setup_kcsan_blacklist"}, frames...)
	output, err := osutil.RunCmd(10*time.Minute, "", fuzzer.config.Executor, args...)
	if err != nil {
		log.Fatalf("failed to set KCSAN blacklist: %v", err)
	}
	log.Logf(0, "%s", output)
}

func (fuzzer *Fuzzer) pollLoop() {
	var execTotal uint64
	var lastPoll time.Time
	var lastPrint time.Time
	ticker := time.NewTicker(3 * time.Second).C
	for {
		poll := false
		select {
		case <-ticker:
		case <-fuzzer.needPoll:
			poll = true
		}
		if fuzzer.outputType != OutputStdout && time.Since(lastPrint) > 10*time.Second {
			// Keep-alive for manager.
			log.Logf(0, "alive, executed %v", execTotal)
			lastPrint = time.Now()
		}
		if poll || time.Since(lastPoll) > 10*time.Second {
			needCandidates := fuzzer.workQueue.wantCandidates()
			needTriages := fuzzer.fuzzerConfig.syncTriage && fuzzer.workQueue.wantTriages()
			if poll && !needCandidates {
				continue
			}
			stats := make(map[string]uint64)
			for _, proc := range fuzzer.procs {
				stats["exec total"] += atomic.SwapUint64(&proc.env.StatExecs, 0)
				stats["executor restarts"] += atomic.SwapUint64(&proc.env.StatRestarts, 0)
			}
			for stat := Stat(0); stat < StatCount; stat++ {
				v := atomic.SwapUint64(&fuzzer.stats[stat], 0)
				stats[statNames[stat]] = v
				execTotal += v
			}
			if !fuzzer.poll(needCandidates, needTriages, stats) {
				lastPoll = time.Now()
			}
		}
	}
}

func (fuzzer *Fuzzer) poll(needCandidates bool, needTriages bool, stats map[string]uint64) bool {
	ts0 := time.Now().UnixNano()
	defer func() {
		ts1 := time.Now().UnixNano()
		fuzzer.writeLog("- MAB Poll: %v\n", ts1-ts0)
	}()
	if fuzzer.fuzzerConfig.MABVerbose {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fuzzer.writeLog("- Mem Alloc: %v TotalAlloc: %v Sys: %v NumGC: %v\n", m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC)
	}
	a := &rpctype.PollArgs{
		Name:              fuzzer.name,
		NeedCandidates:    needCandidates,
		NeedTriages:       needTriages,
		MaxSignal:         fuzzer.grabNewSignal().Serialize(),
		Stats:             stats,
		Triages:           fuzzer.triages,
		TriagesUnfinished: make([]rpctype.RPCTriage, 0),
		SmashesFinished:   make([]hash.Sig, 0),
	}

	var mabOverhead int64 = 0
	if fuzzer.fuzzerConfig.MABAlgorithm != "N/A" || fuzzer.fuzzerConfig.MABSeedSelection != "N/A" || fuzzer.fuzzerConfig.syncSmash {
		fuzzer.MABMu.Lock()
		a.RPCMABStatus, mabOverhead = fuzzer.getMABStatus()
	}
	if fuzzer.fuzzerConfig.syncTriage {
		// Send unfinished triages in batches
		count := 5
		for sig, _ := range fuzzer.triagesUnfinished {
			if len(fuzzer.triagesUnfinished[sig]) > count {
				a.TriagesUnfinished = append(a.TriagesUnfinished, fuzzer.triagesUnfinished[sig][:count]...)
				fuzzer.writeLog("- Sending %v triages from program %v\n", count, sig.String())
				count = 0
				fuzzer.triagesUnfinished[sig] = fuzzer.triagesUnfinished[sig][count:]
				break
			} else {
				a.TriagesUnfinished = append(a.TriagesUnfinished, fuzzer.triagesUnfinished[sig]...)
				fuzzer.writeLog("- Sending %v triages from program %v\n", len(fuzzer.triagesUnfinished[sig]), sig.String())
				count -= len(fuzzer.triagesUnfinished[sig])
				delete(fuzzer.triagesUnfinished, sig)
				if count <= 0 {
					break
				}
			}
		}
	}
	if fuzzer.fuzzerConfig.syncSmash {
		// Send finished smashes in batches
		if len(fuzzer.smashesFinished) > 5 {
			a.SmashesFinished = fuzzer.smashesFinished[:5]
			fuzzer.smashesFinished = fuzzer.smashesFinished[5:]
		} else {
			a.SmashesFinished = fuzzer.smashesFinished
			fuzzer.smashesFinished = make([]hash.Sig, 0)
		}
		fuzzer.writeLog("- Sending %v smashes.\n", len(a.SmashesFinished))
	}

	r := &rpctype.PollRes{
		RPCMABStatus: rpctype.RPCMABStatus{
			// CorpusGLC:  make(map[hash.Sig]glc.CorpusGLC),
			// TriageInfo: make(map[hash.Sig]*glc.TriageInfo),
		},
	}
	if fuzzer.fuzzerConfig.MABVerbose {
		fuzzer.writeLog("- %s\n", "Calling Manager")
	}
	if err := fuzzer.manager.Call("Manager.Poll", a, r); err != nil {
		log.Fatalf("Manager.Poll call failed: %v", err)
	}
	if fuzzer.fuzzerConfig.MABVerbose {
		fuzzer.writeLog("- %s\n", "Success")
	}
	maxSignal := r.MaxSignal.Deserialize()
	log.Logf(1, "poll: candidates=%v inputs=%v signal=%v",
		len(r.Candidates), len(r.NewInputs), maxSignal.Len())
	fuzzer.addMaxSignal(maxSignal)
	for _, inp := range r.NewInputs {
		fuzzer.addInputFromAnotherFuzzer(inp)
	}
	for _, candidate := range r.Candidates {
		p, err := fuzzer.target.Deserialize(candidate.Prog, prog.NonStrict)
		if err != nil {
			log.Fatalf("failed to parse program from manager: %v", err)
		}
		flags := ProgCandidate
		if candidate.Minimized {
			flags |= ProgMinimized
		}
		if candidate.Smashed {
			flags |= ProgSmashed
		}
		fuzzer.workQueue.enqueue(&WorkCandidate{
			p:     p,
			flags: flags,
		})
	}
	if needCandidates && len(r.Candidates) == 0 && atomic.LoadUint32(&fuzzer.triagedCandidates) == 0 {
		atomic.StoreUint32(&fuzzer.triagedCandidates, 1)
	}

	if fuzzer.fuzzerConfig.syncTriage {
		// Delete finished triages
		toBeDeleted := make([]hash.Sig, 0)
		for sig, v := range fuzzer.triages {
			if v == 1 {
				toBeDeleted = append(toBeDeleted, sig)
			}
		}
		for _, sig := range toBeDeleted {
			delete(fuzzer.triages, sig)
			fuzzer.writeLog("- Completed triages from program %v. %v progs remaining\n", sig.String(), len(fuzzer.triages))
		}
		// Fetch unfinished triages
		if needTriages {
			for _, tri := range r.Triages {
				p, err := fuzzer.target.Deserialize(tri.Prog, prog.NonStrict)
				if err != nil {
					fuzzer.writeLog("- WTF failed to deserialize prog from another fuzzer: %v", err)
					continue
				}
				fuzzer.triages[tri.Sig] = 0
				fuzzer.workQueue.enqueue(&WorkTriage{
					p:     p.Clone(),
					call:  tri.CallIndex,
					info:  tri.Info,
					flags: ProgTypes(tri.Flags),
				})
				fuzzer.writeLog("- Adding triage work from manager Prog=%v (%v, %v), CallIndex=%v #Sig=%v\n", tri.Sig.String(), p.CorpusGLC.Cost, p.CorpusGLC.MutateCount, tri.CallIndex, len(tri.Info.Signal))
			}
		}
	}
	if fuzzer.fuzzerConfig.MABAlgorithm != "N/A" || fuzzer.fuzzerConfig.MABSeedSelection != "N/A" || fuzzer.fuzzerConfig.syncSmash {
		mabOverhead += fuzzer.writeMABStatus(r.RPCMABStatus)
		fuzzer.MABMu.Unlock()
		fuzzer.writeLog("- MAB Sync: %v\n", mabOverhead)
	}

	return len(r.NewInputs) != 0 || len(r.Candidates) != 0 || maxSignal.Len() != 0
}

func (fuzzer *Fuzzer) newTriage(inp rpctype.RPCTriage) {
	if !fuzzer.fuzzerConfig.syncTriage {
		return
	}
	ts0 := time.Now().UnixNano()
	defer func() {
		ts1 := time.Now().UnixNano()
		fuzzer.writeLog("- MAB NewTriage: %v\n", ts1-ts0)
	}()
	// Update local record
	fuzzer.triages[inp.Sig] = 0
	// Update local buffer for sync
	if _, ok := fuzzer.triagesUnfinished[inp.Sig]; !ok {
		fuzzer.triagesUnfinished[inp.Sig] = make([]rpctype.RPCTriage, 0)
	}
	fuzzer.triagesUnfinished[inp.Sig] = append(fuzzer.triagesUnfinished[inp.Sig], inp)
	// Triage Info for corpus target
	if fuzzer.fuzzerConfig.MABTargetCorpus {
		if _, ok := fuzzer.MABTriageInfo[inp.Sig]; !ok {
			fuzzer.MABTriageInfo[inp.Sig] = &glc.TriageInfo{}
		}
		fuzzer.MABTriageInfo[inp.Sig].TriageTotal += 1
		fuzzer.MABTriageInfo[inp.Sig].Source = inp.Source
		fuzzer.MABTriageInfo[inp.Sig].SourceCost = inp.SourceCost
		fuzzer.writeLog("- MAB NewTriageInfo %+v\n", fuzzer.MABTriageInfo[inp.Sig])
	}
}

func (fuzzer *Fuzzer) completeTriage(inp rpctype.RPCTriage) {
	if !fuzzer.fuzzerConfig.syncTriage {
		return
	}
	ts0 := time.Now().UnixNano()
	defer func() {
		ts1 := time.Now().UnixNano()
		fuzzer.writeLog("- MAB CompleteTriage: %v\n", ts1-ts0)
	}()
	// Delete local record
	if _, ok := fuzzer.triages[inp.Sig]; ok {
		fuzzer.triages[inp.Sig] = 1
	}
	if _, ok := fuzzer.triagesUnfinished[inp.Sig]; ok {
		delete(fuzzer.triagesUnfinished, inp.Sig)
	}
}

func (fuzzer *Fuzzer) sendInputToManager(inp rpctype.RPCInput) {
	a := &rpctype.NewInputArgs{
		Name:     fuzzer.name,
		RPCInput: inp,
	}
	if err := fuzzer.manager.Call("Manager.NewInput", a, nil); err != nil {
		log.Fatalf("Manager.NewInput call failed: %v", err)
	}
}

func (fuzzer *Fuzzer) addInputFromAnotherFuzzer(inp rpctype.RPCInput) {
	p, err := fuzzer.target.Deserialize(inp.Prog, prog.NonStrict)
	if err != nil {
		log.Fatalf("failed to deserialize prog from another fuzzer: %v", err)
	}
	// Sync MAB status
	p.CorpusGLC = inp.CorpusGLC
	/*
		p.MABMutateCount = inp.MutateCount
		p.MABVerifyGain = inp.VerifyGain
		p.MABVerifyCost = inp.VerifyCost
		p.MABMinimizeGain = inp.MinimizeGain
		p.MABMinimizeCost = inp.MinimizeCost
		p.MABMinimizeTimeSave = inp.MinimizeTimeSave
		p.MABMutateCost = inp.MutateCost
		p.MABMutateGain = inp.MutateGain
		p.MABMutateGainNorm = inp.MutateGainNorm
		p.MABMutateGainNormOrig = inp.MutateGainNormOrig
		p.MABTriageGainNorm = inp.TriageGainNorm
		p.MABCostBeforeMinimize = inp.CostBeforeMinimize
		p.Smashed = inp.Smashed
	*/

	sig := hash.Hash(inp.Prog)
	sign := inp.Signal.Deserialize()
	if fuzzer.fuzzerConfig.MABVerbose {
		fuzzer.writeLog("- addInputFromAnotherFuzzer: %v, %+v\n", sig.String(), p.CorpusGLC)
	}
	fuzzer.addInputToCorpus(p, sign, sig)

	if fuzzer.fuzzerConfig.syncSmash && (!p.CorpusGLC.Smashed || p.CorpusGLC.MutateCount-fuzzer.fuzzerConfig.smashWeight < 0) {
		// Add smashing work to workqueue
		numSmashes := fuzzer.fuzzerConfig.smashWeight - p.CorpusGLC.MutateCount
		numMutates := fuzzer.fuzzerConfig.mutateWeight
		for numSmashes > 0 {
			n := numMutates
			if n > numSmashes {
				n = numSmashes
			}
			// Note: the second parameter: call is only useful for fault injection and hint seed. We just ignore this for now to speed up the syncing
			fuzzer.workQueue.enqueue(&WorkSmash{p, -1, n})
			numSmashes -= numMutates
		}

	}
}

func (fuzzer *FuzzerSnapshot) chooseProgram(r *rand.Rand) (int, *prog.Prog) {
	randVal := 0.0
	randVal = r.Float64() * fuzzer.sumPrios
	pidx := sort.Search(len(fuzzer.corpusPriosSum), func(i int) bool {
		return fuzzer.corpusPriosSum[i] >= randVal
	})
	if fuzzer.fuzzerConfig.MABVerbose {
		if len(fuzzer.corpusPrios) > 10 {
			fmt.Printf("- Corpus Priority %v, %v...%v\n", fuzzer.corpusPrios[pidx], fuzzer.corpusPrios[:5], fuzzer.corpusPrios[(len(fuzzer.corpusPrios)-5):])
			fmt.Printf("- Corpus Priority Sum %v, %v...%v, %v\n", fuzzer.corpusPriosSum[pidx], fuzzer.corpusPriosSum[:5], fuzzer.corpusPriosSum[(len(fuzzer.corpusPrios)-5):], fuzzer.sumPrios)
		} else {
			fmt.Printf("- Corpus Priority %v\n", fuzzer.corpusPrios)
			fmt.Printf("- Corpus Priority Sum %v, %v\n", fuzzer.corpusPriosSum, fuzzer.sumPrios)
		}
	}
	if pidx >= len(fuzzer.corpus) {
		pidx = len(fuzzer.corpus) - 1
		fmt.Printf("- Error. chooseProgram out of bound. %v/%v\n", pidx, len(fuzzer.corpus))
	}
	return pidx, fuzzer.corpus[pidx]
}

func (fuzzer *Fuzzer) addInputToCorpus(p *prog.Prog, sign signal.Signal, sig hash.Sig) int {
	pidx := -1 // If duplicate seed, do not set pidx
	fuzzer.corpusMu.Lock()
	_, ok := fuzzer.corpusHashes[sig]
	if !ok {
		prio := float64(len(sign))
		if sign.Empty() {
			prio = 1.0
		} else if strings.Contains(fuzzer.fuzzerConfig.MABSeedSelection, "Exp3") {
			prio = math.Exp(fuzzer.MABCorpusEta * p.CorpusGLC.MutateGainNormOrig)
		}
		fuzzer.corpus = append(fuzzer.corpus, p)
		pidx = len(fuzzer.corpus) - 1
		fuzzer.corpusHashes[sig] = pidx
		fuzzer.sumPrios += prio
		fuzzer.corpusPrios = append(fuzzer.corpusPrios, prio)
		fuzzer.corpusPriosSum = append(fuzzer.corpusPriosSum, fuzzer.sumPrios)
	}
	fuzzer.corpusMu.Unlock()

	if !sign.Empty() {
		fuzzer.signalMu.Lock()
		fuzzer.corpusSignal.Merge(sign)
		fuzzer.maxSignal.Merge(sign)
		fuzzer.signalMu.Unlock()
	}
	return pidx
}

func (fuzzer *Fuzzer) snapshot() FuzzerSnapshot {
	fuzzer.corpusMu.RLock()
	defer fuzzer.corpusMu.RUnlock()
	return FuzzerSnapshot{&fuzzer.fuzzerConfig, fuzzer.corpus, fuzzer.corpusPrios, fuzzer.corpusPriosSum, fuzzer.sumPrios, fuzzer.workQueue}
}

func (fuzzer *Fuzzer) addMaxSignal(sign signal.Signal) {
	if sign.Len() == 0 {
		return
	}
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	fuzzer.maxSignal.Merge(sign)
}

func (fuzzer *Fuzzer) grabNewSignal() signal.Signal {
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	sign := fuzzer.newSignal
	if sign.Empty() {
		return nil
	}
	fuzzer.newSignal = nil
	return sign
}

func (fuzzer *Fuzzer) corpusSignalDiff(sign signal.Signal) signal.Signal {
	fuzzer.signalMu.RLock()
	defer fuzzer.signalMu.RUnlock()
	return fuzzer.corpusSignal.Diff(sign)
}

func (fuzzer *Fuzzer) checkNewSignal(p *prog.Prog, info *ipc.ProgInfo) (calls []int, extra bool, gain float64) {
	fuzzer.signalMu.RLock()
	defer fuzzer.signalMu.RUnlock()
	gain = 0.0
	thisGain := 0.0
	for i, inf := range info.Calls {
		thisGain = fuzzer.checkNewCallSignal(p, &inf, i)
		if thisGain > 0 {
			calls = append(calls, i)
			gain += thisGain
		}
	}
	extra = false
	thisGain = fuzzer.checkNewCallSignal(p, &info.Extra, -1)
	if thisGain > 0 {
		extra = true
		gain += thisGain
	}
	return
}

func (fuzzer *Fuzzer) logSignal(sign signal.Signal, prefix string) {
	for e := range sign {
		fuzzer.writeLog("%s %x\n", prefix, e)
	}
}

func (fuzzer *Fuzzer) checkNewCallSignal(p *prog.Prog, info *ipc.CallInfo, call int) float64 {
	diff := fuzzer.maxSignal.DiffRaw(info.Signal, signalPrio(p, info, call))
	fuzzer.writeLog("- %v\n", call)
	fuzzer.logSignal(diff, "=")
	if diff.Empty() {
		return 0.0
	}
	fuzzer.signalMu.RUnlock()
	fuzzer.signalMu.Lock()
	fuzzer.maxSignal.Merge(diff)
	fuzzer.newSignal.Merge(diff)
	fuzzer.signalMu.Unlock()
	fuzzer.signalMu.RLock()
	return float64(len(diff))
}

func signalPrio(p *prog.Prog, info *ipc.CallInfo, call int) (prio uint8) {
	if call == -1 {
		return 0
	}
	if info.Errno == 0 {
		prio |= 1 << 1
	}
	if !p.Target.CallContainsAny(p.Calls[call]) {
		prio |= 1 << 0
	}
	return
}

func parseOutputType(str string) OutputType {
	switch str {
	case "none":
		return OutputNone
	case "stdout":
		return OutputStdout
	case "dmesg":
		return OutputDmesg
	case "file":
		return OutputFile
	default:
		log.Fatalf("-output flag must be one of none/stdout/dmesg/file")
		return OutputNone
	}
}
