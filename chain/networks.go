package chain

import "strconv"

var chains = map[uint]string{
	1:  "Foundation",
	2:  "Morden",
	3:  "Ropsten",
	4:  "Rinkeby",
	5:  "Goerli",
	42: "Kovan",
}

// ResolveNetworkID returns the name of the network
// or the string of the id if it is not found
func ResolveNetworkID(id uint) string {
	n, ok := chains[id]
	if ok {
		return n
	}

	return strconv.Itoa(int(id))
}
