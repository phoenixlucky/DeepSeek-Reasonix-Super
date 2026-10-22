// Package peermail is the on-disk half of messages between sessions of one user
// on one machine: a record of which runtimes are live, and an immutable spool of
// messages addressed to each. It decides nothing about delivery. The receiving
// controller reads its own spool, holds what it finds for the user, and admits
// what the user approves through its own inbox; the sender never writes into
// the receiver's transcript, queue or prefix.
//
// Everything lives under one private directory beside the sessions. A runtime
// is live only while the caller's own liveness check says so (the session
// lease), so a leftover file describes a crash, never a peer. A message names
// no authority: it carries text, a sender identity and a hop count, and nothing
// that approves, elevates or widens anything in the receiver.
//
// The directory is outside every write root and in the forbidden-read list
// (config.Roots.PeersDir), which is what makes a sender's identity and a
// message's confidentiality hold against a confined command. This package
// checks the directory is private to the user and refuses to follow a symlink.
package peermail
