from seven_evaluate import processor
from seven_evaluate.reducer import reduce

@processor(id="browser-summary", version="2.0.0")
def summarize(context):
    output = reduce(context)
    if context["phase"] == "finalize":
        output["processor"] = "browser-summary@2.0.0"
        output["data"]["fixture_label"] = "自定义 Processor 输出"
    return output
