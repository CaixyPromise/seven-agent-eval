from seven_evaluate import processor
from seven_evaluate.basic import process


@processor(id="browser-summary", version="1.0.0")
def summarize(context):
    output = process(context)
    output["processor"] = "browser-summary@1.0.0"
    output["data"]["fixture_label"] = "自定义 Processor 输出"
    return output
