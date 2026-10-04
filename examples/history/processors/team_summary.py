from seven_evaluate import processor

@processor(id="team-summary", version="2.0.0")
def summarize(ctx):
    if ctx["phase"] == "initialize": return {"count":0}
    if ctx["phase"] == "update": return {"count":ctx["state"]["count"]+len(ctx["cases"])}
    return {"processor":"team-summary@2.0.0","input_digest":ctx["input_digest"],
            "data":{"team_note":"自定义处理器已替换默认统计","sample_count":ctx["state"]["count"]}}
