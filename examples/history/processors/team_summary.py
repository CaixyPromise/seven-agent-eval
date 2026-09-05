from seven_evaluate import processor

@processor(id="team-summary", version="1.0.0")
def summarize(context):
    return {"processor": "team-summary@1.0.0",
            "input_digest": context["input_digest"],
            "data": {"team_note": "自定义处理器已替换默认统计", "sample_count": len(context["cases"])}}
