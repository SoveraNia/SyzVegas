config_bases = [
{'test_name':'KCOV_Default', 'feedback':"KCOV", 'fuzzer_config':{}},
# Regular
{'test_name':'KCOV_MAB', 'feedback':"KCOV", 'fuzzer_config':{
        "generateWeight": 1, 'mutateWeight': 1, 'smashWeight': 20,
        "MABTriageFirst": False, "MABNormalize": 500, "MABTimeAverage": 1000000,
        "MABAlgorithm": "Exp3-GC", "MABSeedSelection": "N/A",
        "MABVerbose": True, "MABGenerateFirst": -1, "MABAssocTriageGain": 1.0,
        "MABResetMax": -1, "MABResetWeight": -1, "MABDuration": -1}},
{'test_name':'KCOV_MAB-Window', 'feedback':"KCOV", 'fuzzer_config':{
        "generateWeight": 1, 'mutateWeight': 1, 'smashWeight': 20,
        "MABTriageFirst": False, "MABNormalize": 500, "MABTimeAverage": 1000000,
        "MABAlgorithm": "Exp3-GC", "MABSeedSelection": "N/A",
        "MABVerbose": True, "MABGenerateFirst": -1, "MABAssocTriageGain": 1.0,
        "MABResetMax": -1, "MABResetWeight": -1, "MABDuration": -1}},
# Seed selection
{'test_name':'KCOV_MAB-SS-Gain', 'feedback':"KCOV", 'fuzzer_config':{
        "generateWeight": 1, 'mutateWeight': 1, 'smashWeight': 20,
        "MABTriageFirst": False, "MABNormalize": 0, "MABTimeAverage": 1000000,
        "MABAlgorithm": "Exp3-GC", "MABSeedSelection": "Exp3-Gain",
        "MABVerbose": True, "MABGenerateFirst": -1, "MABAssocTriageGain": 1.0,
        "MABResetMax": -1, "MABResetWeight": -1, "MABDuration": -1}},
{'test_name':'KCOV_MAB-SS-GC', 'feedback':"KCOV", 'fuzzer_config':{
        "generateWeight": 1, 'mutateWeight': 1, 'smashWeight': 20,
        "MABTriageFirst": False, "MABNormalize": 0, "MABTimeAverage": 1000000,
        "MABAlgorithm": "Exp3-GC", "MABSeedSelection": "Exp3-GC",
        "MABVerbose": True, "MABGenerateFirst": -1, "MABAssocTriageGain": 1.0,
        "MABResetMax": -1, "MABResetWeight": -1, "MABDuration": -1}},
{'test_name':'KCOV_MAB-SS-Gain-Window', 'feedback':"KCOV", 'fuzzer_config':{
        "generateWeight": 1, 'mutateWeight': 1, 'smashWeight': 20,
        "MABTriageFirst": False, "MABNormalize": 500, "MABTimeAverage": 1000000,
        "MABAlgorithm": "Exp3-GC", "MABSeedSelection": "Exp3-Gain",
        "MABVerbose": True, "MABGenerateFirst": -1, "MABAssocTriageGain": 1.0,
        "MABResetMax": -1, "MABResetWeight": -1, "MABDuration": -1}},
{'test_name':'KCOV_MAB-SS-GC-Window', 'feedback':"KCOV", 'fuzzer_config':{
        "generateWeight": 1, 'mutateWeight': 1, 'smashWeight': 20,
        "MABTriageFirst": False, "MABNormalize": 500, "MABTimeAverage": 1000000,
        "MABAlgorithm": "Exp3-GC", "MABSeedSelection": "Exp3-GC",
        "MABVerbose": True, "MABGenerateFirst": -1, "MABAssocTriageGain": 1.0,
        "MABResetMax": -1, "MABResetWeight": -1, "MABDuration": -1}},

]

