import os
import subprocess
import traceback
import socket

FILE_DIR = os.path.dirname(os.path.realpath(__file__))
SYZKALLER_DIR = os.path.join(FILE_DIR, "..")
CUR_DIR = os.path.abspath(os.path.curdir)

def SHELL(cmd, permissive=False):
    try:
        ret = subprocess.check_call(cmd, shell=True)
        return ret;
    except subprocess.CalledProcessError as exc:
        if not permissive:
            traceback.print_exc()
            exit(1)
        return exc.returncode

def getOpenPort():
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.bind(("", 0))
    s.listen(1)
    port = s.getsockname()[1]
    s.close()
    return port

'''
def filterLog(infile, outfile):
    buf = b''
    lc = 0
    with open(outfile, "w+") as fout:
      with open(infile, "rb") as fin:
        byte = fin.read(1)
        while byte:
          buf += byte
          if byte == b'\n':
            try:
              lc += 1
              if lc % 100000 == 0:
                print("\n%s: Parsed %d lines" % (infile, lc))
              s = buf.decode()
              if s[0] != '[':
                fout.write(s)
            except:
              pass
            buf = b''
          byte = fin.read(1)
'''

def filterLog(infile, outfile):
    lc = 0
    first_byte = True
    log_byte = False
    with open(outfile, "wb+") as fout:
      with open(infile, "rb") as fin:
        byte = fin.read(1)
        while byte:
          if byte == b'\n':
            lc += 1
            if lc % 100000 == 0:
              print("\n%s: Parsed %d lines" % (infile, lc))
            if log_byte:
              fout.write(byte)
            first_byte = True
            log_byte = False
          elif first_byte:
            if byte in [b'#', b'>', b'<', b'-', b'=']:
              fout.write(byte)
              log_byte = True
            first_byte = False
          elif log_byte:
            fout.write(byte)
          byte = fin.read(1)

def setEnv():
    os.environ["GOPATH"] = os.path.join(os.path.dirname(os.path.realpath(__file__)),"..","..","..","..","..")
    if os.path.isdir("/extra/dwang030/go/bin"):
        os.environ["PATH"] = "/extra/dwang030/go/bin:" + os.environ["PATH"]
    elif os.path.isdir("/home/dwang030/go/bin"):
        os.environ["PATH"] = "/home/dwang030/go/bin:" + os.environ["PATH"]
    elif os.path.isdir("/usr/local/go/bin"):
        os.environ["PATH"] = "/usr/local/go/bin:" + os.environ["PATH"]
    else:
        print("Error: Cannot find Go binary")
        exit(1)
    if os.path.exists("/usr/bin/clang-format"):
        os.environ["CLANGFORMAT"] = "/usr/bin/clang-format"
        os.environ["PATH"] = "/usr/bin:" + os.environ["PATH"]
    elif os.path.exists("/usr/bin/clang-format-7"):
        os.environ["CLANGFORMAT"] = "/usr/bin/clang-format-7"
    elif os.path.exists("/home/dwang030/clang+llvm/bin/clang-format"):
        os.environ["CLANGFORMAT"] = "/home/dwang030/clang+llvm/bin/clang-format"
        os.environ["PATH"] = "/home/dwang030/clang+llvm/bin:" + os.environ["PATH"]
    elif os.path.exists("/extra/dwang030/clang+llvm/bin/clang-format"):
        os.environ["CLANGFORMAT"] = "/extra/dwang030/clang+llvm/bin/clang-format"
        os.environ["PATH"] = "/extra/dwang030/clang+llvm/bin:" + os.environ["PATH"]

