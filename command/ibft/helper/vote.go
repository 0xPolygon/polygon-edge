package helper

type Vote string

const (
	VoteAdd    = "ADD"
	VoteRemove = "REMOVE"
)

func BoolToVote(vote bool) Vote {
	if vote {
		return VoteAdd
	}

	return VoteRemove
}

func VoteToString(vote Vote) string {
	return string(vote)
}
