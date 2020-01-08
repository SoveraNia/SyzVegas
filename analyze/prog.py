import copy
import traceback

class Prog:
    allProgs = []
    knownProgs = {}

    def __init__(self, calls, ts=0, signal=0, origin=None):
        self.id = -1;
        self.ts = ts;
        self.calls = copy.deepcopy(calls);
        self.signal = signal
        self.origin = None
        self.childrenMinimize = []
        # inaccurate arg count and size
        self.argCount = 0
        self.argSize = 0
        for call in self.calls:
            try:
                argv = call.split(' ', 1)[1]
            except:
                traceback.print_exc()
                print(call)
                exit(1)
            self.argSize += len(argv)
            if argv != '()':
                self.argCount += (argv.count(", ") + 1)
    
    @staticmethod
    def newProg(calls, ts=0, signal=0, origin=None):
        str_calls = '\n'.join(calls)
        if str_calls in Prog.knownProgs:
            i = Prog.knownProgs[str_calls].id
            return Prog.allProgs[i]
        p = Prog(calls, signal, origin)
        p.id = len(Prog.allProgs)
        Prog.allProgs.append(p)
        Prog.knownProgs[str_calls] = p
        return p
    def toDict(self):
        ret = {}
        for k in self.__dict__.keys():
            if k == "childrenMinimize":
                ret[k] = [];
                for p in self.childrenMinimize:
                     ret[k].append(p.id)
            else:
                ret[k] = self.__dict__[k]
        return ret
    def __str__(self):
        return str(self.toDict())
    def __eq__(self, other):
        return self.calls == other.calls
    def __hash__(self):
        return hash(self.calls)
    def __len__(self):
        return len(self.calls)

