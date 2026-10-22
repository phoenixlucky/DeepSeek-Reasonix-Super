package i18n

// GoalMessages is how the terminal footer words a goal's state and usage; Messages embeds it.
type GoalMessages struct {
	ChatStatusGoalLabel    string
	ChatStatusGoalFmt      string // state, turns, tokens, work time
	ChatStatusGoalRunning  string
	ChatStatusGoalComplete string
	ChatStatusGoalBlocked  string
	ChatStatusGoalStopped  string
}
