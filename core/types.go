/**
* Copyright (c) 2018 EMSECO 
* This source code is being disclosed to you solely for the purpose of your participation in 
* testing EMSECO. You may view, compile and run the code for that purpose and pursuant to 
* the protocols and algorithms that are programmed into, and intended by, the code. You may 
* not do anything else with the code without express permission from EMSECO Foundation Ltd., 
* including modifying or publishing the code (or any part of it), and developing or forming 
* another public or private blockchain network. This source code is provided ‘as is’ and no 
* warranties are given as to title or non-infringement, merchantability or fitness for purpose 
* and, to the extent permitted by law, all liability for your use of the code is disclaimed. 
* Some programs in this code are governed by the GNU General Public License v3.0 (available at 
* https://www.gnu.org/licenses/gpl-3.0.en.html) (‘GPLv3’). The programs that are governed by 
* GPLv3.0 are those programs that are located in the folders src/depends and tests/depends 
* and which include a reference to GPLv3 in their program files.
**/

package core

import (
	"encoding/hex"
	"math/big"

	"go-hashgrid/core/common"
	"go-hashgrid/core/hexutil"
	"go-hashgrid/crypto/sha3"
	"go-hashgrid/rlp"
)

const (
	HashLength       = 32
	AddressLength    = 20
	EventDuration    = 1000
	OverflowDuration = 5000
	MaxTransInEvent  = 64
)

type Hash [HashLength]byte
type Address [AddressLength]byte

func (a *Address) Compare(b *Address) int {
	la := len(a)
	lb := len(b)

	if la > lb {
		return 1
	}
	if la < lb {
		return -1
	}

	for i := 0; i < la; i++ {
		if a[i] != b[i] {
			if a[i] > b[i] {
				return 1
			}
			if a[i] < b[i] {
				return -1
			}
		}
	}
	return 0
}

func (a *Hash) Compare(b *Hash) int {
	la := len(a)
	lb := len(b)

	if la > lb {
		return 1
	}
	if la < lb {
		return -1
	}

	for i := 0; i < la; i++ {
		if a[i] != b[i] {
			if a[i] > b[i] {
				return 1
			}
			if a[i] < b[i] {
				return -1
			}
		}
	}
	return 0
}

// SetBytes sets the hash to the value of b.
// If b is larger than len(h), b will be cropped from the left.
func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-HashLength:]
	}

	copy(h[HashLength-len(b):], b)
}

// BytesToHash sets b to hash.
// If b is larger than len(h), b will be cropped from the left.
func BytesToHash(b []byte) Hash {
	var h Hash
	h.SetBytes(b)
	return h
}

// BigToHash sets byte representation of b to hash.
// If b is larger than len(h), b will be cropped from the left.
func BigToHash(b *big.Int) Hash { return BytesToHash(b.Bytes()) }

// HexToHash sets byte representation of s to hash.
// If b is larger than len(h), b will be cropped from the left.
func HexToHash(s string) Hash { return BytesToHash(common.FromHex(s)) }

// Bytes gets the byte representation of the underlying hash.
func (h Hash) Bytes() []byte { return h[:] }

// Big converts a hash to a big integer.
func (h Hash) Big() *big.Int { return new(big.Int).SetBytes(h[:]) }

// Hex converts a hash to a hex string.
func (h Hash) Hex() string { return hexutil.Encode(h[:]) }

// BytesToAddress returns Address with value b.
// If b is larger than len(h), b will be cropped from the left.
func BytesToAddress(b []byte) Address {
	var a Address
	a.SetBytes(b)
	return a
}

// BigToAddress returns Address with byte values of b.
// If b is larger than len(h), b will be cropped from the left.
func BigToAddress(b *big.Int) Address { return BytesToAddress(b.Bytes()) }

// HexToAddress returns Address with byte values of s.
// If s is larger than len(h), s will be cropped from the left.
func HexToAddress(s string) Address { return BytesToAddress(common.FromHex(s)) }

// IsHexAddress verifies whether a string can represent a valid hex-encoded
// Ethereum address or not.
func IsHexAddress(s string) bool {
	if common.HasHexPrefix(s) {
		s = s[2:]
	}
	return len(s) == 2*AddressLength && common.IsHex(s)
}

// Bytes gets the string representation of the underlying address.
func (a Address) Bytes() []byte { return a[:] }

// Big converts an address to a big integer.
func (a Address) Big() *big.Int { return new(big.Int).SetBytes(a[:]) }

// Hash converts an address to a hash by left-padding it with zeros.
func (a Address) Hash() Hash { return BytesToHash(a[:]) }

// Hex returns an EIP55-compliant hex string representation of the address.
func (a Address) Hex() string {
	unchecksummed := hex.EncodeToString(a[:])
	sha := sha3.NewKeccak256()
	sha.Write([]byte(unchecksummed))
	hash := sha.Sum(nil)

	result := []byte(unchecksummed)
	for i := 0; i < len(result); i++ {
		hashByte := hash[i/2]
		if i%2 == 0 {
			hashByte = hashByte >> 4
		} else {
			hashByte &= 0xf
		}
		if result[i] > '9' && hashByte > 7 {
			result[i] -= 32
		}
	}
	return "0x" + string(result)
}

// SetBytes sets the address to the value of b.
// If b is larger than len(a) it will panic.
func (a *Address) SetBytes(b []byte) {
	if len(b) > len(a) {
		b = b[len(b)-AddressLength:]
	}
	copy(a[AddressLength-len(b):], b)
}

type Transaction struct {
}

type GlobalConfig struct {
	InterMilli int
	Addr       string
	Coinbase   Address
	Peers      []string
}

type Account struct {
	Addr Address
}

type Sig struct {
	Addr Address
}

type Event struct {
	TimeStamp  *big.Int
	Trans      []*Transaction
	SelfPart   Hash
	SelfHash   Hash
	SelfSig    Sig
	OtherPartH Hash
	OtherPartA Address
	RoundID    *big.Int
}

type RequestArg struct {
	TargetHash Hash
	TargetAddr Address
}

func CopyEvent(e *Event) *Event {
	cpy := *e
	if cpy.Trans = make([]*Transaction, 0, MaxTransInEvent); cpy.Trans != nil {
		copy(cpy.Trans, e.Trans)
	}
	return &cpy
}

func (e *Event) Hash() Hash {
	return rlpHash([]interface{}{
		e.TimeStamp,
		e.Trans,
		e.SelfPart,
		e.SelfSig,
		e.OtherPartH,
	})
}

func rlpHash(x interface{}) (h Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])

	return h
}
