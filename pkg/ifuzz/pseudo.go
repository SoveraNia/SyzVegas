// Copyright 2017 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package ifuzz

import (
	"math/rand"
)

func initPseudo() {
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_RDMSR",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			msr := msrs[r.Intn(len(msrs))]
			gen.mov32(regECX, msr)
			gen.byte(0x0f, 0x32) // rdmsr
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_WRMSR",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			msr := msrs[r.Intn(len(msrs))]
			v := generateInt(cfg, r, 8)
			gen.mov32(regECX, msr)
			gen.mov32(regEAX, uint32(v>>0))
			gen.mov32(regEDX, uint32(v>>32))
			gen.byte(0x0f, 0x30) // wrmsr
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_PCI_READ",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			addr, port, size := pciAddrPort(r)
			gen.out32(0xcf8, addr)
			gen.in(port, size)
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_PCI_WRITE",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			addr, port, size := pciAddrPort(r)
			val := generateInt(cfg, r, 4)
			gen.out32(0xcf8, addr)
			gen.out(port, uint32(val), size)
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_PORT_READ",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			port := ports[r.Intn(len(ports))]
			gen.in(port, r.Intn(3))
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_PORT_WRITE",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			port := ports[r.Intn(len(ports))]
			val := generateInt(cfg, r, 4)
			gen.out(port, uint32(val), r.Intn(3))
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_XOR_CR",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			cr := controlRegisters[r.Intn(len(controlRegisters))]
			var v uint32
			if cr == 8 {
				v = uint32(r.Intn(15) + 1)
			} else {
				bit := controlRegistersBits[cr][r.Intn(len(controlRegistersBits[cr]))]
				v = 1 << bit
			}
			gen.readCR(cr)
			gen.xor32(regEAX, v)
			gen.writeCR(cr)
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_XOR_EFER",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			gen.mov32(regECX, eferMSR)
			gen.byte(0x0f, 0x32) // rdmsr
			bit := eferBits[r.Intn(len(eferBits))]
			gen.xor32(regEAX, 1<<bit)
			gen.byte(0x0f, 0x30) // wrmsr
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_SET_BREAK",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			br := uint8(r.Intn(4))
			loc := uint32(r.Intn(4))
			typ := uint32(r.Intn(16))
			addr := generateInt(cfg, r, 8)
			if cfg.Mode == ModeLong64 {
				gen.mov64(regRAX, addr)
			} else {
				gen.mov32(regEAX, uint32(addr))
			}
			gen.writeDR(br)
			gen.readDR(7)
			gen.xor32(regEAX, loc<<(br*2)|typ<<(16+br*4))
			gen.writeDR(7)
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_LOAD_SEG",
		Mode:   1<<ModeLast - 1,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			sel := randSelector(r)
			if cfg.Mode == ModeReal16 {
				sel = uint16(generateInt(cfg, r, 8)) >> 4
			}
			reg := uint8(r.Intn(6))
			gen.mov16(regAX, sel)
			gen.byte(0x8e, 0xc0|(reg<<3)) // mov %ax, %seg
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_FAR_JMP",
		Mode:   1<<ModeLong64 | 1<<ModeProt32 | 1<<ModeProt16,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			sel := randSelector(r)
			off := generateInt(cfg, r, 4)
			if cfg.Mode == ModeLong64 {
				gen.mov32toSPaddr(uint32(sel), 0)
				gen.mov32toSPaddr(uint32(off), 2)
				if r.Intn(2) == 0 {
					gen.byte(0xff, 0x2c, 0x24) // ljmp (%rsp)
				} else {
					gen.byte(0xff, 0x1c, 0x24) // lcall (%rsp)
				}
			} else {
				if r.Intn(2) == 0 {
					gen.byte(0xea) // ljmp $imm16, $imm16/32
				} else {
					gen.byte(0x9a) // lcall $imm16, $imm16/32
				}
				if cfg.Mode == ModeProt16 {
					gen.imm16(uint16(off))
				} else {
					gen.imm32(uint32(off))
				}
				gen.imm16(sel)
			}
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_LTR_LLDT",
		Mode:   1<<ModeLong64 | 1<<ModeProt32 | 1<<ModeProt16,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			sel := randSelector(r)
			gen.mov16(regAX, sel)
			if r.Intn(2) == 0 {
				gen.byte(0x0f, 0x00, 0xd8) // ltr %ax
			} else {
				gen.byte(0x0f, 0x00, 0xd0) // lldt %ax
			}
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_LGIDT",
		Mode:   1<<ModeLong64 | 1<<ModeProt32 | 1<<ModeProt16,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			limit := uint32(generateInt(cfg, r, 2))
			base := uint32(generateInt(cfg, r, 4))
			gen.mov32toSPaddr(limit, 0)
			gen.mov32toSPaddr(base, 2)
			gen.mov32toSPaddr(0, 6)
			gen.addr32()
			if r.Intn(2) == 0 {
				gen.byte(0x0f, 0x01, 0x14, 0x24) // lgdt (%rsp)
			} else {
				gen.byte(0x0f, 0x01, 0x1c, 0x24) // lidt (%rsp)
			}
			return gen.text
		},
	})
	Insns = append(Insns, &Insn{
		Name:   "PSEUDO_HYPERCALL",
		Mode:   1<<ModeLong64 | 1<<ModeProt32 | 1<<ModeProt16,
		Priv:   true,
		Pseudo: true,
		generator: func(cfg *Config, r *rand.Rand) []byte {
			gen := makeGen(cfg, r)
			switch r.Intn(2) {
			case 0:
				gen.mov32(regEAX, 1) // KVM_HC_VAPIC_POLL_IRQ
			case 1:
				gen.mov32(regEAX, 5)                              // KVM_HC_KICK_CPU
				gen.mov32(regECX, uint32(generateInt(cfg, r, 4))) // APIC ID
			default:
				panic("bad")
			}
			if r.Intn(2) == 0 {
				gen.byte(0x0f, 0x01, 0xd9) // vmmcall
			} else {
				gen.byte(0x0f, 0x01, 0xc1) // vmcall
			}
			return gen.text
		},
	})
}

const (
	regAL = iota
	regAX
	regEAX
	regRAX
	regCL
	regCX
	regECX
	regRCX
	regDL
	regDX
	regEDX
	regRDX
)

type generator struct {
	mode int
	r    *rand.Rand
	text []byte
}

func makeGen(cfg *Config, r *rand.Rand) *generator {
	return &generator{
		mode: cfg.Mode,
		r:    r,
	}
}

func (gen *generator) byte(v ...uint8) {
	gen.text = append(gen.text, v...)
}

func (gen *generator) imm16(v uint16) {
	gen.byte(byte(v>>0), byte(v>>8))
}

func (gen *generator) imm32(v uint32) {
	gen.byte(byte(v>>0), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (gen *generator) imm64(v uint64) {
	gen.byte(byte(v>>0), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func (gen *generator) operand16() {
	switch gen.mode {
	case ModeLong64, ModeProt32:
		gen.byte(0x66)
	case ModeProt16, ModeReal16:
	default:
		panic("bad mode")
	}
}

func (gen *generator) operand32() {
	switch gen.mode {
	case ModeLong64, ModeProt32:
	case ModeProt16, ModeReal16:
		gen.byte(0x66)
	default:
		panic("bad mode")
	}
}

func (gen *generator) addr32() {
	switch gen.mode {
	case ModeLong64, ModeProt32:
	case ModeProt16, ModeReal16:
		gen.byte(0x67)
	default:
		panic("bad mode")
	}
}

func (gen *generator) mov8(reg int, v uint8) {
	switch reg {
	case regAL:
		gen.byte(0xb0)
	case regCL:
		gen.byte(0xb1)
	case regDL:
		gen.byte(0xb2)
	default:
		panic("unknown register")
	}
	gen.byte(v)
}

func (gen *generator) mov16(reg int, v uint16) {
	gen.operand16()
	switch reg {
	case regAX:
		gen.byte(0xb8)
	case regCX:
		gen.byte(0xb9)
	case regDX:
		gen.byte(0xba)
	default:
		panic("unknown register")
	}
	gen.imm16(v)
}

func (gen *generator) mov32(reg int, v uint32) {
	gen.operand32()
	switch reg {
	case regEAX:
		gen.byte(0xb8)
	case regECX:
		gen.byte(0xb9)
	case regEDX:
		gen.byte(0xba)
	default:
		panic("unknown register")
	}
	gen.imm32(v)
}

func (gen *generator) mov64(reg int, v uint64) {
	if gen.mode != ModeLong64 {
		panic("bad mode")
	}
	gen.byte(0x48)
	switch reg {
	case regRAX:
		gen.byte(0xb8)
	case regRCX:
		gen.byte(0xb9)
	case regRDX:
		gen.byte(0xba)
	default:
		panic("unknown register")
	}
	gen.imm64(v)
}

// movl $v, off(%rsp)
func (gen *generator) mov32toSPaddr(v uint32, off uint8) {
	gen.addr32()
	gen.operand32()
	gen.byte(0xc7, 0x44, 0x24, off)
	gen.imm32(v)
}

func (gen *generator) xor32(reg int, v uint32) {
	gen.operand32()
	switch reg {
	case regEAX:
		gen.byte(0x35)
	default:
		panic("unknown register")
	}
	gen.imm32(v)
}

func (gen *generator) readCR(cr uint8) {
	if cr < 8 {
		// mov %crN, %eax/%rax
		gen.byte(0x0f, 0x20, 0xc0|cr<<3)
	} else if cr < 16 {
		// mov %crN, %eax/%rax
		gen.byte(0x44, 0x0f, 0x20, 0xc0|(cr-8)<<3)
	} else {
		panic("bad cr")
	}
}

func (gen *generator) writeCR(cr uint8) {
	if cr < 8 {
		// mov %eax/%rax, %crN
		gen.byte(0x0f, 0x22, 0xc0|cr<<3)
	} else if cr < 16 {
		// mov %eax/%rax, %crN
		gen.byte(0x44, 0x0f, 0x22, 0xc0|(cr-8)<<3)
	} else {
		panic("bad cr")
	}
}

func (gen *generator) readDR(dr uint8) {
	if dr >= 8 {
		panic("bad dr")
	}
	// mov %drN, %eax/%rax
	gen.byte(0x0f, 0x21, 0xc0|dr<<3)
}

func (gen *generator) writeDR(dr uint8) {
	if dr >= 8 {
		panic("bad dr")
	}
	// mov %eax/%rax, %drN
	gen.byte(0x0f, 0x23, 0xc0|dr<<3)
}

func (gen *generator) in8(port uint16) {
	gen.mov16(regDX, port)
	gen.byte(0xec) // in %al, %dx
}

func (gen *generator) in16(port uint16) {
	gen.mov16(regDX, port)
	gen.operand16()
	gen.byte(0xed) // in %ax, %dx
}

func (gen *generator) in32(port uint16) {
	gen.mov16(regDX, port)
	gen.operand32()
	gen.byte(0xed) // in %eax, %dx
}

func (gen *generator) in(port uint16, size int) {
	switch size {
	case 0:
		gen.in8(port)
	case 1:
		gen.in16(port)
	case 2:
		gen.in32(port)
	default:
		panic("bad size")
	}
}

func (gen *generator) out8(port uint16, v uint8) {
	gen.mov16(regDX, port)
	gen.mov8(regAL, v)
	gen.byte(0xee) // out %dx, %al
}

func (gen *generator) out16(port uint16, v uint16) {
	gen.mov16(regDX, port)
	gen.mov16(regAX, v)
	gen.operand16()
	gen.byte(0xef) // out %dx, %ax
}

func (gen *generator) out32(port uint16, v uint32) {
	gen.mov16(regDX, port)
	gen.mov32(regEAX, v)
	gen.operand32()
	gen.byte(0xef) // out %dx, %eax
}

func (gen *generator) out(port uint16, v uint32, size int) {
	switch size {
	case 0:
		gen.out8(port, uint8(v))
	case 1:
		gen.out16(port, uint16(v))
	case 2:
		gen.out32(port, v)
	default:
		panic("bad size")
	}
}

func randSelector(r *rand.Rand) uint16 {
	seg := uint16(r.Intn(40))
	dpl := uint16(r.Intn(4))
	ldt := uint16(r.Intn(2))
	return seg<<3 | ldt<<2 | dpl
}

func pciAddrPort(r *rand.Rand) (addr uint32, port uint16, size int) {
	bus := uint32(r.Intn(256))
	dev := uint32(r.Intn(32))
	fn := uint32(r.Intn(8))
	reghi := uint32(r.Intn(16))
	reglo := uint32(r.Intn(64)) << 2
	port = 0xcfc
	switch size = r.Intn(3); size {
	case 0:
		port += uint16(reglo & 3)
		reglo += uint32(r.Intn(4))
	case 1:
		port += uint16(reglo & 2)
		reglo += uint32(r.Intn(2) * 2)
	case 2:
	}
	addr = 0x80000000 | reghi<<24 | bus<<16 | dev<<11 | fn<<8 | reglo
	return
}

var controlRegisters = []uint8{0, 3, 4, 8}
var controlRegistersBits = map[uint8][]uint8{
	0: {0, 1, 2, 3, 4, 5, 16, 18, 29, 30, 31},
	3: {3, 5},
	4: {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 13, 14, 16, 17, 18, 20, 21, 22},
}

const eferMSR = 0xC0000080

var eferBits = []uint8{0, 8, 10, 11, 12, 13, 14, 15}

var ports = []uint16{
	0x40, 0x41, 0x42, 0x43, // PIT
	0x61,                                 // speaker
	0x20, 0x21, 0xa0, 0xa1, 0x4d0, 0x4d1, // 8259
}

// sys/kvm.txt also knows this list
var msrs = []uint32{
	0x0, 0x1, 0x10, 0x11, 0x12, 0x13, 0x17, 0x1b,
	0x20, 0x21, 0x28, 0x29, 0x2a, 0x2c, 0x33, 0x34,
	0x3a, 0x3b, 0x40, 0x60, 0x79, 0x88, 0x89, 0x8a,
	0x8b, 0x9b, 0x9e, 0xc1, 0xc2, 0xcd, 0xce, 0xe2,
	0xe7, 0xe8, 0xfe, 0x116, 0x118, 0x119, 0x11a, 0x11b,
	0x11e, 0x174, 0x175, 0x176, 0x179, 0x17a, 0x17b, 0x180,
	0x181, 0x182, 0x183, 0x184, 0x185, 0x186, 0x187, 0x188,
	0x189, 0x18a, 0x198, 0x199, 0x19a, 0x19b, 0x19c, 0x19d,
	0x1a0, 0x1a2, 0x1a6, 0x1a7, 0x1aa, 0x1ad, 0x1ae, 0x1af,
	0x1b0, 0x1b1, 0x1b2, 0x1c8, 0x1c9, 0x1d9, 0x1db, 0x1dc,
	0x1dd, 0x1de, 0x1e0, 0x1fc, 0x200, 0x201, 0x202, 0x203,
	0x204, 0x205, 0x206, 0x207, 0x208, 0x209, 0x20a, 0x20b,
	0x20c, 0x20d, 0x20e, 0x20f, 0x210, 0x211, 0x212, 0x213,
	0x214, 0x215, 0x216, 0x217, 0x218, 0x219, 0x21a, 0x21b,
	0x21c, 0x21d, 0x21e, 0x21f, 0x220, 0x221, 0x222, 0x223,
	0x224, 0x225, 0x226, 0x227, 0x228, 0x229, 0x22a, 0x22b,
	0x22c, 0x22d, 0x22e, 0x22f, 0x230, 0x231, 0x232, 0x233,
	0x234, 0x235, 0x236, 0x237, 0x238, 0x239, 0x23a, 0x23b,
	0x23c, 0x23d, 0x23e, 0x23f, 0x240, 0x241, 0x242, 0x243,
	0x244, 0x245, 0x246, 0x247, 0x248, 0x249, 0x24a, 0x24b,
	0x24c, 0x24d, 0x24e, 0x24f, 0x250, 0x251, 0x252, 0x253,
	0x254, 0x255, 0x256, 0x257, 0x258, 0x259, 0x25a, 0x25b,
	0x25c, 0x25d, 0x25e, 0x25f, 0x260, 0x261, 0x262, 0x263,
	0x264, 0x265, 0x266, 0x267, 0x268, 0x269, 0x26a, 0x26b,
	0x26c, 0x26d, 0x26e, 0x26f, 0x270, 0x271, 0x272, 0x273,
	0x274, 0x275, 0x276, 0x277, 0x278, 0x279, 0x27a, 0x27b,
	0x27c, 0x27d, 0x27e, 0x27f, 0x280, 0x281, 0x282, 0x283,
	0x284, 0x285, 0x286, 0x287, 0x288, 0x289, 0x28a, 0x28b,
	0x28c, 0x28d, 0x28e, 0x28f, 0x290, 0x291, 0x292, 0x293,
	0x294, 0x295, 0x296, 0x297, 0x298, 0x299, 0x29a, 0x29b,
	0x29c, 0x29d, 0x29e, 0x29f, 0x2a0, 0x2a1, 0x2a2, 0x2a3,
	0x2a4, 0x2a5, 0x2a6, 0x2a7, 0x2a8, 0x2a9, 0x2aa, 0x2ab,
	0x2ac, 0x2ad, 0x2ae, 0x2af, 0x2b0, 0x2b1, 0x2b2, 0x2b3,
	0x2b4, 0x2b5, 0x2b6, 0x2b7, 0x2b8, 0x2b9, 0x2ba, 0x2bb,
	0x2bc, 0x2bd, 0x2be, 0x2bf, 0x2c0, 0x2c1, 0x2c2, 0x2c3,
	0x2c4, 0x2c5, 0x2c6, 0x2c7, 0x2c8, 0x2c9, 0x2ca, 0x2cb,
	0x2cc, 0x2cd, 0x2ce, 0x2cf, 0x2d0, 0x2d1, 0x2d2, 0x2d3,
	0x2d4, 0x2d5, 0x2d6, 0x2d7, 0x2d8, 0x2d9, 0x2da, 0x2db,
	0x2dc, 0x2dd, 0x2de, 0x2df, 0x2e0, 0x2e1, 0x2e2, 0x2e3,
	0x2e4, 0x2e5, 0x2e6, 0x2e7, 0x2e8, 0x2e9, 0x2ea, 0x2eb,
	0x2ec, 0x2ed, 0x2ee, 0x2ef, 0x2f0, 0x2f1, 0x2f2, 0x2f3,
	0x2f4, 0x2f5, 0x2f6, 0x2f7, 0x2f8, 0x2f9, 0x2fa, 0x2fb,
	0x2fc, 0x2fd, 0x2fe, 0x2ff, 0x300, 0x301, 0x302, 0x303,
	0x304, 0x305, 0x306, 0x307, 0x308, 0x309, 0x30a, 0x30b,
	0x30c, 0x30d, 0x30e, 0x30f, 0x310, 0x311, 0x312, 0x313,
	0x314, 0x315, 0x316, 0x317, 0x318, 0x319, 0x31a, 0x31b,
	0x31c, 0x31d, 0x31e, 0x31f, 0x320, 0x321, 0x322, 0x323,
	0x324, 0x325, 0x326, 0x327, 0x328, 0x329, 0x32a, 0x32b,
	0x32c, 0x32d, 0x32e, 0x32f, 0x330, 0x331, 0x332, 0x333,
	0x334, 0x335, 0x336, 0x337, 0x338, 0x339, 0x33a, 0x33b,
	0x33c, 0x33d, 0x33e, 0x33f, 0x340, 0x341, 0x342, 0x343,
	0x344, 0x345, 0x346, 0x347, 0x348, 0x349, 0x34a, 0x34b,
	0x34c, 0x34d, 0x34e, 0x34f, 0x350, 0x351, 0x352, 0x353,
	0x354, 0x355, 0x356, 0x357, 0x358, 0x359, 0x35a, 0x35b,
	0x35c, 0x35d, 0x35e, 0x35f, 0x360, 0x361, 0x362, 0x363,
	0x364, 0x365, 0x366, 0x367, 0x368, 0x369, 0x36a, 0x36b,
	0x36c, 0x36d, 0x36e, 0x36f, 0x370, 0x371, 0x372, 0x373,
	0x374, 0x375, 0x376, 0x377, 0x378, 0x379, 0x37a, 0x37b,
	0x37c, 0x37d, 0x37e, 0x37f, 0x380, 0x381, 0x382, 0x383,
	0x384, 0x385, 0x386, 0x387, 0x388, 0x389, 0x38a, 0x38b,
	0x38c, 0x38d, 0x38e, 0x38f, 0x390, 0x391, 0x392, 0x393,
	0x394, 0x395, 0x396, 0x397, 0x398, 0x399, 0x39a, 0x39b,
	0x39c, 0x39d, 0x39e, 0x39f, 0x3a0, 0x3a1, 0x3a2, 0x3a3,
	0x3a4, 0x3a5, 0x3a6, 0x3a7, 0x3a8, 0x3a9, 0x3aa, 0x3ab,
	0x3ac, 0x3ad, 0x3ae, 0x3af, 0x3b0, 0x3b1, 0x3b2, 0x3b3,
	0x3b4, 0x3b5, 0x3b6, 0x3b7, 0x3b8, 0x3b9, 0x3ba, 0x3bb,
	0x3bc, 0x3bd, 0x3be, 0x3bf, 0x3c2, 0x3c3, 0x3c4, 0x3c5,
	0x3f1, 0x3f2, 0x3f6, 0x3f7, 0x3f8, 0x3f9, 0x3fa, 0x3fc,
	0x3fd, 0x3fe, 0x3ff, 0x400, 0x401, 0x402, 0x403, 0x404,
	0x405, 0x406, 0x407, 0x408, 0x409, 0x40a, 0x40b, 0x40c,
	0x40d, 0x40e, 0x40f, 0x410, 0x411, 0x412, 0x413, 0x480,
	0x481, 0x482, 0x483, 0x484, 0x485, 0x486, 0x487, 0x488,
	0x489, 0x48a, 0x48b, 0x48c, 0x48d, 0x48e, 0x48f, 0x490,
	0x491, 0x4c1, 0x4d0, 0x560, 0x561, 0x570, 0x571, 0x572,
	0x580, 0x581, 0x582, 0x583, 0x584, 0x585, 0x586, 0x587,
	0x600, 0x606, 0x60a, 0x60b, 0x60c, 0x60d, 0x610, 0x611,
	0x613, 0x614, 0x618, 0x619, 0x61b, 0x61c, 0x630, 0x631,
	0x632, 0x633, 0x634, 0x635, 0x638, 0x639, 0x63a, 0x63b,
	0x640, 0x641, 0x642, 0x648, 0x649, 0x64a, 0x64b, 0x64c,
	0x64d, 0x64e, 0x64f, 0x658, 0x659, 0x65a, 0x65b, 0x660,
	0x668, 0x669, 0x680, 0x690, 0x6b0, 0x6b1, 0x6c0, 0x6e0,
	0x770, 0x771, 0x772, 0x773, 0x774, 0x777, 0x800, 0x801,
	0x802, 0x803, 0x804, 0x805, 0x806, 0x807, 0x808, 0x809,
	0x80a, 0x80b, 0x80c, 0x80d, 0x80e, 0x80f, 0x810, 0x811,
	0x812, 0x813, 0x814, 0x815, 0x816, 0x817, 0x818, 0x819,
	0x81a, 0x81b, 0x81c, 0x81d, 0x81e, 0x81f, 0x820, 0x821,
	0x822, 0x823, 0x824, 0x825, 0x826, 0x827, 0x828, 0x829,
	0x82a, 0x82b, 0x82c, 0x82d, 0x82e, 0x82f, 0x830, 0x831,
	0x832, 0x833, 0x834, 0x835, 0x836, 0x837, 0x838, 0x839,
	0x83a, 0x83b, 0x83c, 0x83d, 0x83e, 0x83f, 0x840, 0x841,
	0x842, 0x843, 0x844, 0x845, 0x846, 0x847, 0x848, 0x849,
	0x84a, 0x84b, 0x84c, 0x84d, 0x84e, 0x84f, 0x850, 0x851,
	0x852, 0x853, 0x854, 0x855, 0x856, 0x857, 0x858, 0x859,
	0x85a, 0x85b, 0x85c, 0x85d, 0x85e, 0x85f, 0x860, 0x861,
	0x862, 0x863, 0x864, 0x865, 0x866, 0x867, 0x868, 0x869,
	0x86a, 0x86b, 0x86c, 0x86d, 0x86e, 0x86f, 0x870, 0x871,
	0x872, 0x873, 0x874, 0x875, 0x876, 0x877, 0x878, 0x879,
	0x87a, 0x87b, 0x87c, 0x87d, 0x87e, 0x87f, 0x880, 0x881,
	0x882, 0x883, 0x884, 0x885, 0x886, 0x887, 0x888, 0x889,
	0x88a, 0x88b, 0x88c, 0x88d, 0x88e, 0x88f, 0x890, 0x891,
	0x892, 0x893, 0x894, 0x895, 0x896, 0x897, 0x898, 0x899,
	0x89a, 0x89b, 0x89c, 0x89d, 0x89e, 0x89f, 0x8a0, 0x8a1,
	0x8a2, 0x8a3, 0x8a4, 0x8a5, 0x8a6, 0x8a7, 0x8a8, 0x8a9,
	0x8aa, 0x8ab, 0x8ac, 0x8ad, 0x8ae, 0x8af, 0x8b0, 0x8b1,
	0x8b2, 0x8b3, 0x8b4, 0x8b5, 0x8b6, 0x8b7, 0x8b8, 0x8b9,
	0x8ba, 0x8bb, 0x8bc, 0x8bd, 0x8be, 0x8bf, 0x8c0, 0x8c1,
	0x8c2, 0x8c3, 0x8c4, 0x8c5, 0x8c6, 0x8c7, 0x8c8, 0x8c9,
	0x8ca, 0x8cb, 0x8cc, 0x8cd, 0x8ce, 0x8cf, 0x8d0, 0x8d1,
	0x8d2, 0x8d3, 0x8d4, 0x8d5, 0x8d6, 0x8d7, 0x8d8, 0x8d9,
	0x8da, 0x8db, 0x8dc, 0x8dd, 0x8de, 0x8df, 0x8e0, 0x8e1,
	0x8e2, 0x8e3, 0x8e4, 0x8e5, 0x8e6, 0x8e7, 0x8e8, 0x8e9,
	0x8ea, 0x8eb, 0x8ec, 0x8ed, 0x8ee, 0x8ef, 0x8f0, 0x8f1,
	0x8f2, 0x8f3, 0x8f4, 0x8f5, 0x8f6, 0x8f7, 0x8f8, 0x8f9,
	0x8fa, 0x8fb, 0x8fc, 0x8fd, 0x8fe, 0x8ff, 0x900, 0x901,
	0x902, 0x903, 0x904, 0x905, 0x906, 0x907, 0x908, 0x909,
	0x90a, 0x90b, 0x90c, 0x90d, 0x90e, 0x90f, 0x910, 0x911,
	0x912, 0x913, 0x914, 0x915, 0x916, 0x917, 0x918, 0x919,
	0x91a, 0x91b, 0x91c, 0x91d, 0x91e, 0x91f, 0x920, 0x921,
	0x922, 0x923, 0x924, 0x925, 0x926, 0x927, 0x928, 0x929,
	0x92a, 0x92b, 0x92c, 0x92d, 0x92e, 0x92f, 0x930, 0x931,
	0x932, 0x933, 0x934, 0x935, 0x936, 0x937, 0x938, 0x939,
	0x93a, 0x93b, 0x93c, 0x93d, 0x93e, 0x93f, 0x940, 0x941,
	0x942, 0x943, 0x944, 0x945, 0x946, 0x947, 0x948, 0x949,
	0x94a, 0x94b, 0x94c, 0x94d, 0x94e, 0x94f, 0x950, 0x951,
	0x952, 0x953, 0x954, 0x955, 0x956, 0x957, 0x958, 0x959,
	0x95a, 0x95b, 0x95c, 0x95d, 0x95e, 0x95f, 0x960, 0x961,
	0x962, 0x963, 0x964, 0x965, 0x966, 0x967, 0x968, 0x969,
	0x96a, 0x96b, 0x96c, 0x96d, 0x96e, 0x96f, 0x970, 0x971,
	0x972, 0x973, 0x974, 0x975, 0x976, 0x977, 0x978, 0x979,
	0x97a, 0x97b, 0x97c, 0x97d, 0x97e, 0x97f, 0x980, 0x981,
	0x982, 0x983, 0x984, 0x985, 0x986, 0x987, 0x988, 0x989,
	0x98a, 0x98b, 0x98c, 0x98d, 0x98e, 0x98f, 0x990, 0x991,
	0x992, 0x993, 0x994, 0x995, 0x996, 0x997, 0x998, 0x999,
	0x99a, 0x99b, 0x99c, 0x99d, 0x99e, 0x99f, 0x9a0, 0x9a1,
	0x9a2, 0x9a3, 0x9a4, 0x9a5, 0x9a6, 0x9a7, 0x9a8, 0x9a9,
	0x9aa, 0x9ab, 0x9ac, 0x9ad, 0x9ae, 0x9af, 0x9b0, 0x9b1,
	0x9b2, 0x9b3, 0x9b4, 0x9b5, 0x9b6, 0x9b7, 0x9b8, 0x9b9,
	0x9ba, 0x9bb, 0x9bc, 0x9bd, 0x9be, 0x9bf, 0x9c0, 0x9c1,
	0x9c2, 0x9c3, 0x9c4, 0x9c5, 0x9c6, 0x9c7, 0x9c8, 0x9c9,
	0x9ca, 0x9cb, 0x9cc, 0x9cd, 0x9ce, 0x9cf, 0x9d0, 0x9d1,
	0x9d2, 0x9d3, 0x9d4, 0x9d5, 0x9d6, 0x9d7, 0x9d8, 0x9d9,
	0x9da, 0x9db, 0x9dc, 0x9dd, 0x9de, 0x9df, 0x9e0, 0x9e1,
	0x9e2, 0x9e3, 0x9e4, 0x9e5, 0x9e6, 0x9e7, 0x9e8, 0x9e9,
	0x9ea, 0x9eb, 0x9ec, 0x9ed, 0x9ee, 0x9ef, 0x9f0, 0x9f1,
	0x9f2, 0x9f3, 0x9f4, 0x9f5, 0x9f6, 0x9f7, 0x9f8, 0x9f9,
	0x9fa, 0x9fb, 0x9fc, 0x9fd, 0x9fe, 0x9ff, 0xa00, 0xa01,
	0xa02, 0xa03, 0xa04, 0xa05, 0xa06, 0xa07, 0xa08, 0xa09,
	0xa0a, 0xa0b, 0xa0c, 0xa0d, 0xa0e, 0xa0f, 0xa10, 0xa11,
	0xa12, 0xa13, 0xa14, 0xa15, 0xa16, 0xa17, 0xa18, 0xa19,
	0xa1a, 0xa1b, 0xa1c, 0xa1d, 0xa1e, 0xa1f, 0xa20, 0xa21,
	0xa22, 0xa23, 0xa24, 0xa25, 0xa26, 0xa27, 0xa28, 0xa29,
	0xa2a, 0xa2b, 0xa2c, 0xa2d, 0xa2e, 0xa2f, 0xa30, 0xa31,
	0xa32, 0xa33, 0xa34, 0xa35, 0xa36, 0xa37, 0xa38, 0xa39,
	0xa3a, 0xa3b, 0xa3c, 0xa3d, 0xa3e, 0xa3f, 0xa40, 0xa41,
	0xa42, 0xa43, 0xa44, 0xa45, 0xa46, 0xa47, 0xa48, 0xa49,
	0xa4a, 0xa4b, 0xa4c, 0xa4d, 0xa4e, 0xa4f, 0xa50, 0xa51,
	0xa52, 0xa53, 0xa54, 0xa55, 0xa56, 0xa57, 0xa58, 0xa59,
	0xa5a, 0xa5b, 0xa5c, 0xa5d, 0xa5e, 0xa5f, 0xa60, 0xa61,
	0xa62, 0xa63, 0xa64, 0xa65, 0xa66, 0xa67, 0xa68, 0xa69,
	0xa6a, 0xa6b, 0xa6c, 0xa6d, 0xa6e, 0xa6f, 0xa70, 0xa71,
	0xa72, 0xa73, 0xa74, 0xa75, 0xa76, 0xa77, 0xa78, 0xa79,
	0xa7a, 0xa7b, 0xa7c, 0xa7d, 0xa7e, 0xa7f, 0xa80, 0xa81,
	0xa82, 0xa83, 0xa84, 0xa85, 0xa86, 0xa87, 0xa88, 0xa89,
	0xa8a, 0xa8b, 0xa8c, 0xa8d, 0xa8e, 0xa8f, 0xa90, 0xa91,
	0xa92, 0xa93, 0xa94, 0xa95, 0xa96, 0xa97, 0xa98, 0xa99,
	0xa9a, 0xa9b, 0xa9c, 0xa9d, 0xa9e, 0xa9f, 0xaa0, 0xaa1,
	0xaa2, 0xaa3, 0xaa4, 0xaa5, 0xaa6, 0xaa7, 0xaa8, 0xaa9,
	0xaaa, 0xaab, 0xaac, 0xaad, 0xaae, 0xaaf, 0xab0, 0xab1,
	0xab2, 0xab3, 0xab4, 0xab5, 0xab6, 0xab7, 0xab8, 0xab9,
	0xaba, 0xabb, 0xabc, 0xabd, 0xabe, 0xabf, 0xac0, 0xac1,
	0xac2, 0xac3, 0xac4, 0xac5, 0xac6, 0xac7, 0xac8, 0xac9,
	0xaca, 0xacb, 0xacc, 0xacd, 0xace, 0xacf, 0xad0, 0xad1,
	0xad2, 0xad3, 0xad4, 0xad5, 0xad6, 0xad7, 0xad8, 0xad9,
	0xada, 0xadb, 0xadc, 0xadd, 0xade, 0xadf, 0xae0, 0xae1,
	0xae2, 0xae3, 0xae4, 0xae5, 0xae6, 0xae7, 0xae8, 0xae9,
	0xaea, 0xaeb, 0xaec, 0xaed, 0xaee, 0xaef, 0xaf0, 0xaf1,
	0xaf2, 0xaf3, 0xaf4, 0xaf5, 0xaf6, 0xaf7, 0xaf8, 0xaf9,
	0xafa, 0xafb, 0xafc, 0xafd, 0xafe, 0xaff, 0xb00, 0xb01,
	0xb02, 0xb03, 0xb04, 0xb05, 0xb06, 0xb07, 0xb08, 0xb09,
	0xb0a, 0xb0b, 0xb0c, 0xb0d, 0xb0e, 0xb0f, 0xb10, 0xb11,
	0xb12, 0xb13, 0xb14, 0xb15, 0xb16, 0xb17, 0xb18, 0xb19,
	0xb1a, 0xb1b, 0xb1c, 0xb1d, 0xb1e, 0xb1f, 0xb20, 0xb21,
	0xb22, 0xb23, 0xb24, 0xb25, 0xb26, 0xb27, 0xb28, 0xb29,
	0xb2a, 0xb2b, 0xb2c, 0xb2d, 0xb2e, 0xb2f, 0xb30, 0xb31,
	0xb32, 0xb33, 0xb34, 0xb35, 0xb36, 0xb37, 0xb38, 0xb39,
	0xb3a, 0xb3b, 0xb3c, 0xb3d, 0xb3e, 0xb3f, 0xb40, 0xb41,
	0xb42, 0xb43, 0xb44, 0xb45, 0xb46, 0xb47, 0xb48, 0xb49,
	0xb4a, 0xb4b, 0xb4c, 0xb4d, 0xb4e, 0xb4f, 0xb50, 0xb51,
	0xb52, 0xb53, 0xb54, 0xb55, 0xb56, 0xb57, 0xb58, 0xb59,
	0xb5a, 0xb5b, 0xb5c, 0xb5d, 0xb5e, 0xb5f, 0xb60, 0xb61,
	0xb62, 0xb63, 0xb64, 0xb65, 0xb66, 0xb67, 0xb68, 0xb69,
	0xb6a, 0xb6b, 0xb6c, 0xb6d, 0xb6e, 0xb6f, 0xb70, 0xb71,
	0xb72, 0xb73, 0xb74, 0xb75, 0xb76, 0xb77, 0xb78, 0xb79,
	0xb7a, 0xb7b, 0xb7c, 0xb7d, 0xb7e, 0xb7f, 0xb80, 0xb81,
	0xb82, 0xb83, 0xb84, 0xb85, 0xb86, 0xb87, 0xb88, 0xb89,
	0xb8a, 0xb8b, 0xb8c, 0xb8d, 0xb8e, 0xb8f, 0xb90, 0xb91,
	0xb92, 0xb93, 0xb94, 0xb95, 0xb96, 0xb97, 0xb98, 0xb99,
	0xb9a, 0xb9b, 0xb9c, 0xb9d, 0xb9e, 0xb9f, 0xba0, 0xba1,
	0xba2, 0xba3, 0xba4, 0xba5, 0xba6, 0xba7, 0xba8, 0xba9,
	0xbaa, 0xbab, 0xbac, 0xbad, 0xbae, 0xbaf, 0xbb0, 0xbb1,
	0xbb2, 0xbb3, 0xbb4, 0xbb5, 0xbb6, 0xbb7, 0xbb8, 0xbb9,
	0xbba, 0xbbb, 0xbbc, 0xbbd, 0xbbe, 0xbbf, 0xbc0, 0xbc1,
	0xbc2, 0xbc3, 0xbc4, 0xbc5, 0xbc6, 0xbc7, 0xbc8, 0xbc9,
	0xbca, 0xbcb, 0xbcc, 0xbcd, 0xbce, 0xbcf, 0xbd0, 0xbd1,
	0xbd2, 0xbd3, 0xbd4, 0xbd5, 0xbd6, 0xbd7, 0xbd8, 0xbd9,
	0xbda, 0xbdb, 0xbdc, 0xbdd, 0xbde, 0xbdf, 0xbe0, 0xbe1,
	0xbe2, 0xbe3, 0xbe4, 0xbe5, 0xbe6, 0xbe7, 0xbe8, 0xbe9,
	0xbea, 0xbeb, 0xbec, 0xbed, 0xbee, 0xbef, 0xbf0, 0xbf1,
	0xbf2, 0xbf3, 0xbf4, 0xbf5, 0xbf6, 0xbf7, 0xbf8, 0xbf9,
	0xbfa, 0xbfb, 0xbfc, 0xbfd, 0xbfe, 0xbff, 0xd90, 0xda0,
	0xdc0, 0xdc1, 0xdc2, 0xdc3, 0xdc4, 0xdc5, 0xdc6, 0xdc7,
	0x40000000, 0x40000001, 0x40000002, 0x40000003, 0x40000010, 0x40000020, 0x40000022, 0x40000023,
	0x40000070, 0x40000071, 0x40000072, 0x40000073, 0x40000080, 0x40000081, 0x40000082, 0x40000083,
	0x40000084, 0x40000090, 0x40000091, 0x40000092, 0x40000093, 0x40000094, 0x40000095, 0x40000096,
	0x40000097, 0x40000098, 0x40000099, 0x4000009a, 0x4000009b, 0x4000009c, 0x4000009d, 0x4000009e,
	0x4000009f, 0x400000b0, 0x400000b1, 0x400000b2, 0x400000b3, 0x400000b4, 0x400000b5, 0x400000b6,
	0x400000b7, 0x40000100, 0x40000101, 0x40000102, 0x40000103, 0x40000104, 0x40000105, 0x4b564d00,
	0x4b564d01, 0x4b564d02, 0x4b564d03, 0x4b564d04, 0xc0000080, 0xc0000081, 0xc0000082, 0xc0000083,
	0xc0000084, 0xc0000100, 0xc0000101, 0xc0000102, 0xc0000103, 0xc0000104, 0xc001001f, 0xc0010020,
	0xc0010044, 0xc0010062, 0xc0010063, 0xc0010064, 0xc0010114, 0xc0010115, 0xc0010117, 0xc0010140,
	0xc0010141, 0xc0011020, 0xc0011022, 0xc001102a, 0xc0011030, 0xc0011031, 0xc0011032, 0xc0011033,
	0xc0011034, 0xc0011035, 0xc0011036, 0xc0011037, 0xc0011038, 0xc0011039, 0xc001103a, 0xc001103b,
	0xc001103d,
}
