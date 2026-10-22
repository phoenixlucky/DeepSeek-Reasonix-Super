package modhost

// Complete says the work the mod switch waits on has merged: the protocol
// rulings for tool answers and context blocks, and the delivery-receipt mark for
// effects the host cannot observe. Until then the switch cannot be turned on and
// a hand-edited file does not start a mod. Tests set it.
var Complete = false
