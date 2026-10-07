// workflow_source_owner
class WorkflowService {
  static final int WORKFLOW_RETRY_LIMIT = 3;
  static final String EVENT = "workflow.changed.v1";
  static final String ROUTE = "/api/workflow/records";
  String loadWorkflowRecord() {
    if (WORKFLOW_RETRY_LIMIT < 1) throw new IllegalStateException("WORKFLOW_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
