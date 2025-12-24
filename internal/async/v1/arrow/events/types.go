package events

const (
	ArrowAggregateType = "arrow"

	ArrowAddArrowRequestedType = ArrowAggregateType + ".AddArrow.Requested"
	ArrowAddArrowSucceededType = ArrowAggregateType + ".AddArrow.Succeeded"
	ArrowAddArrowFailedType    = ArrowAggregateType + ".AddArrow.Failed"

	ArrowRemoveArrowRequestedType = ArrowAggregateType + ".RemoveArrow.Requested"
	ArrowRemoveArrowSucceededType = ArrowAggregateType + ".RemoveArrow.Succeeded"
	ArrowRemoveArrowFailedType    = ArrowAggregateType + ".RemoveArrow.Failed"

	ArrowExecuteMethodRequestedType       = ArrowAggregateType + ".ExecuteMethod.Requested"
	ArrowExecuteMethodStartedType         = ArrowAggregateType + ".ExecuteMethod.Started"
	ArrowExecuteMethodProgressUpdatedType = ArrowAggregateType + ".ExecuteMethod.ProgressUpdated"
	ArrowExecuteMethodSucceededType       = ArrowAggregateType + ".ExecuteMethod.Succeeded"
	ArrowExecuteMethodFailedType          = ArrowAggregateType + ".ExecuteMethod.Failed"
)
