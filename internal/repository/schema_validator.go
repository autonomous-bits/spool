package repository

import "github.com/autonomous-bits/spool/graphcontract"

type (
	SchemaViolationCode   = graphcontract.SchemaViolationCode
	SchemaViolation       = graphcontract.SchemaViolation
	SchemaValidationError = graphcontract.SchemaValidationError
)

const (
	SchemaViolationInvalidNode          = graphcontract.SchemaViolationInvalidNode
	SchemaViolationInvalidEdge          = graphcontract.SchemaViolationInvalidEdge
	SchemaViolationNodeID               = graphcontract.SchemaViolationNodeID
	SchemaViolationEdgeID               = graphcontract.SchemaViolationEdgeID
	SchemaViolationNodeLabel            = graphcontract.SchemaViolationNodeLabel
	SchemaViolationEdgeType             = graphcontract.SchemaViolationEdgeType
	SchemaViolationRequiredProperty     = graphcontract.SchemaViolationRequiredProperty
	SchemaViolationPropertyType         = graphcontract.SchemaViolationPropertyType
	SchemaViolationMissingSource        = graphcontract.SchemaViolationMissingSource
	SchemaViolationMissingTarget        = graphcontract.SchemaViolationMissingTarget
	SchemaViolationSourceLabel          = graphcontract.SchemaViolationSourceLabel
	SchemaViolationTargetLabel          = graphcontract.SchemaViolationTargetLabel
	SchemaViolationSourceCardinalityMin = graphcontract.SchemaViolationSourceCardinalityMin
	SchemaViolationSourceCardinalityMax = graphcontract.SchemaViolationSourceCardinalityMax
	SchemaViolationTargetCardinalityMin = graphcontract.SchemaViolationTargetCardinalityMin
	SchemaViolationTargetCardinalityMax = graphcontract.SchemaViolationTargetCardinalityMax
	SchemaViolationNaturalKeyUnique     = graphcontract.SchemaViolationNaturalKeyUnique
	SchemaViolationAcyclic              = graphcontract.SchemaViolationAcyclic
	SchemaViolationNoSelfLoop           = graphcontract.SchemaViolationNoSelfLoop
)

var ErrSchemaValidation = graphcontract.ErrSchemaValidation

func ValidateSchemaSnapshot(schema SchemaSnapshot, nodes map[string]Node, edges map[string]Edge) error {
	return graphcontract.ValidateSchemaSnapshot(schema, nodes, edges)
}
