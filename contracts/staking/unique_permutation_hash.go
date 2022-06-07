package staking

import (
	"math/big"
	"sort"
)

type UpHashItem struct {
	index         int
	chaosFactor   *big.Int
	tsFactorical  *big.Int
	probeSequence []int
	key           int
	Up            *UpHash
}

type UpHash struct {
	setSize   int
	keySet    []int
	resultSet []int
	resultNum int
}

func factorial(n int64) *big.Int {
	if n == 0 {
		return big.NewInt(0)
	}

	if n == 1 {
		return big.NewInt(1)
	}

	N := big.NewInt(n)
	return N.Mul(N, factorial(n-1))
}

func NewUpHash(size int) *UpHash {
	set := make([]int, size)
	for i := 0; i < size; i++ {
		set[i] = i
	}
	u := &UpHash{
		setSize: len(set),
		keySet:  set,
	}

	u.resultSet = make([]int, u.setSize)
	for k, _ := range u.resultSet {
		u.resultSet[k] = -1
	}

	return u
}

func (u *UpHash) GenHash(chaosFactor int64) ([]int, error) {
	tsFactorical := factorial(int64(u.setSize))
	chaosFactorBig := new(big.Int).SetInt64(chaosFactor)
	chaosFactorBig = chaosFactorBig.Mod(chaosFactorBig, tsFactorical)
	for _, v := range u.keySet {
		item := NewUpHashItem(0, v, chaosFactorBig, new(big.Int).Set(tsFactorical), u)
		item.UniquePermutationHash(v)
	}

	return u.resultSet, nil
}

func NewUpHashItem(index, key int, chaosFactor, tsFactorical *big.Int, up *UpHash) *UpHashItem {
	return &UpHashItem{
		index:        index,
		chaosFactor:  chaosFactor,
		key:          key,
		tsFactorical: tsFactorical,
		Up:           up,
	}
}

func convert(probe int, sequence []int) int {
	index := 0
	if len(sequence) == 0 {
		return probe
	}
	sort.Ints(sequence)
	// fmt.Println("seq ", sequence)
	for index < len(sequence) && sequence[index] <= probe {
		probe += 1
		index += 1
	}

	return probe
}

func (i *UpHashItem) ProbePermutation() int {
	if i.Up.setSize-i.index <= 0 {
		return -1
	}

	tmpTsFactorical := i.tsFactorical.Div(i.tsFactorical, big.NewInt(int64(i.Up.setSize-i.index)))
	tmpInt := big.NewInt(0)
	probe := tmpInt.Div(i.chaosFactor, tmpTsFactorical).Int64() + 1

	tmpChaosFactor := tmpInt.Mod(i.chaosFactor, tmpTsFactorical)
	nextProbe := convert(int(probe), i.probeSequence)
	i.probeSequence = append(i.probeSequence, nextProbe)
	i.tsFactorical = tmpTsFactorical
	i.chaosFactor = tmpChaosFactor
	i.index += 1

	return nextProbe
}

func (i *UpHashItem) UniquePermutationHash(key int) {
	if i.Up.resultNum == len(i.Up.resultSet) {
		return
	}

	index := i.ProbePermutation()
	if index == -1 {
		return
	}

	if i.Up.resultSet[index-1] == -1 {
		i.Up.resultSet[index-1] = key
		i.Up.resultNum += 1
	} else {
		i.UniquePermutationHash(key)
	}
}
