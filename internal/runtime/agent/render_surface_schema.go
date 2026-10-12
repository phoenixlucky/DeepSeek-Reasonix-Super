package agent

import "encoding/json"

var surfaceSchema = json.RawMessage(`{
"type":"object","additionalProperties":false,"required":["spec_version","body"],
"properties":{"spec_version":{"type":"integer","enum":[1]},"title":{"type":"string","maxLength":200},"body":{"type":"array","minItems":1,"maxItems":200,"items":{"$ref":"#/$defs/node"}}},
"$defs":{
"node":{"type":"object","additionalProperties":false,"required":["kind"],"properties":{
"kind":{"type":"string","enum":["text","markdown","row","stack","kv","meter","pip","divider","field","choice_card"]},
"value":{"type":"string","maxLength":4000},"key":{"type":"string","maxLength":200},"label":{"type":"string","maxLength":200},
"tone":{"type":"string","enum":["dim","strong","ok","warn","err","accent"]},"progress":{"type":"number","minimum":0,"maximum":1},
"children":{"type":"array","items":{"$ref":"#/$defs/node"}},"field":{"$ref":"#/$defs/field"}}},
"field":{"type":"object","additionalProperties":false,"required":["type"],"properties":{
"type":{"type":"string","enum":["string","number","bool","choice","multichoice"]},"required":{"type":"boolean"},
"default":{"anyOf":[{"type":"string"},{"type":"number"},{"type":"boolean"},{"type":"array","items":{"type":"string"}}]},
"min":{"type":"number"},"max":{"type":"number"},"max_length":{"type":"integer","minimum":0,"maximum":4000},
"options":{"type":"array","minItems":1,"maxItems":16,"items":{"$ref":"#/$defs/option"}}}},
"option":{"type":"object","additionalProperties":false,"required":["value","label"],"properties":{
"value":{"type":"string","pattern":"^[a-zA-Z0-9_-]{1,64}$"},"label":{"type":"string","maxLength":200},"description":{"type":"string","maxLength":4000},"preview":{"type":"string","maxLength":4000}}}
}}`)
