from seven_evaluate import evaluator


@evaluator(id="browser-error", version="1.0.0", metrics=["browser.error"])
def evaluate(_context):
    raise RuntimeError("controlled browser fixture evaluator failure")
