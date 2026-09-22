from seven_evaluate import evaluator


@evaluator(
    id="browser-judge",
    version="1.0.0",
    execution_class="judge",
    metrics=["browser.judge"],
)
def evaluate(context):
    case_id = context["case"]["id"]
    return [{
        "id": f"{case_id}:browser.judge",
        "case_id": case_id,
        "metric_id": "browser.judge",
        "status": "observed",
        "value": True,
        "evaluator": "browser-judge@1.0.0",
    }]
