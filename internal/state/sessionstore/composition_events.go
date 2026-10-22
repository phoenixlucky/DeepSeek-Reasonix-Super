package sessionstore

import "reasonix/internal/contract/provider"

const sessionEventCompositionSchemaVersion = 4
const sessionEventLiveCompositionSchemaVersion = 5

func compositionRemoved(previousSchema int, msgs []provider.Message) bool {
	return (previousSchema == sessionEventCompositionSchemaVersion || previousSchema == sessionEventLiveCompositionSchemaVersion) && compositionEventSchema("", msgs, nil) != previousSchema
}

func compositionEventSchema(_ string, msgs []provider.Message, images []sessionEventImages) int {
	for _, m := range msgs {
		if m.AgentComposition != nil {
			if m.AgentComposition.Live() {
				return sessionEventLiveCompositionSchemaVersion
			}
			return sessionEventCompositionSchemaVersion
		}
	}
	return sessionEventRecordSchema(images)
}
