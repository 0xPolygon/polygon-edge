package ibft

type Vote string

const (
	VoteAdd    = "ADD"
	VoteRemove = "REMOVE"
)

func voteToString(vote bool) Vote {
	if vote {
		return VoteAdd
	}

	return VoteRemove
}
