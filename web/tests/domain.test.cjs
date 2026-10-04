const { test } = require("node:test");
const assert = require("node:assert/strict");
const { join } = require("node:path");
const root = process.env.SEVEN_WEB_TEST_BUILD;
const { responseError, errorCopy, safeErrorCode, ProductError } = require(
  join(root, "api/errors.js"),
);
const { assetIdentity, recipeName, isAnomaly, basicSummary, pretty, metricLabel } = require(
  join(root, "domain/presentation.js"),
);
const { evaluationApi } = require(join(root, "api/client.js"));
const { reportKpis } = require(join(root, "domain/reportKpis.js"));

test("custom KPI mapping retains zero/null and detects conflicts without fallback",()=>{
 const output={processor:"team@1",data:{n:0,c:null,"a/b":2},report:{version:"1",kpis:{scored:"/n",coverage:"/c",anomalies:"/a~1b"}}};
 assert.deepEqual(reportKpis([output]),{scored:0,coverage:null,anomalies:2});
 assert.equal(reportKpis([output,output]).conflict,true);
 assert.deepEqual(reportKpis([{processor:"custom",data:{scored:7,coverage:1}}]),{});
 for(const path of ["/missing","/~2","not-pointer"])assert.equal(reportKpis([{...output,report:{version:"1",kpis:{scored:path}}}]).invalid,true);
 for(const n of [false,"0",-1,0.5])assert.equal(reportKpis([{...output,data:{...output.data,n}}]).invalid,true);
});

test("shared builtin paths keep distinct asset row identities", () => {
  const rag={kind:"evaluator",source:"builtin",id:"rag",version:"1",path:"builtin/rag"};
  assert.notEqual(assetIdentity(rag),assetIdentity({...rag,id:"rag-judge"}));
  assert.notEqual(assetIdentity(rag),assetIdentity({...rag,version:"2"}));
  assert.notEqual(assetIdentity(rag),assetIdentity({...rag,path:"evaluators/duplicate.py"}));
});

test("unreviewed backend and upstream messages never become error copy", () => {
  const poison = "secret-token /private/server SQL stack trace";
  for (const payload of [
    { error: poison },
    { code: poison, message: poison },
    null,
    poison,
  ]) {
    const error = responseError(payload, "LOAD_ASSETS");
    assert.equal(error.code, "LOAD_ASSETS");
    assert.ok(!errorCopy[error.code].includes(poison));
  }
  assert.equal(safeErrorCode(new Error(poison), "LOAD_RUN"), "LOAD_RUN");
});
test("known conflict preserves specific safe recovery", () => {
  const error = responseError(
    { code: "RUN_BUSY", error: "hidden" },
    "START_UNKNOWN",
  );
  assert.equal(error.code, "RUN_BUSY");
  assert.match(errorCopy[error.code], /等待完成/);
});
test("omitted, replacement and disabled recipes stay distinct", () => {
  assert.equal(recipeName({ descriptor: {} }), "默认评测");
  assert.equal(
    recipeName({ descriptor: { processors: ["processor:team"] } }),
    "自定义统计",
  );
  assert.equal(
    recipeName({ descriptor: { evaluators: [], processors: [] } }),
    "仅保留结果",
  );
});
test("false and evaluator errors are anomalies; numeric zero is not an error", () => {
  const sample = (value) => ({
    status: "completed",
    metrics: [{ status: "observed", value }],
  });
  assert.equal(isAnomaly(sample(false)), true);
  assert.equal(isAnomaly(sample(0)), false);
  assert.equal(
    isAnomaly({
      status: "completed",
      metrics: [{ status: "error", value: null }],
    }),
    true,
  );
});
test("custom processor is not mistaken for a default summary", () => {
  assert.equal(
    basicSummary({ outputs: [{ processor: "team", data: { coverage: 1 } }] }),
    undefined,
  );
});
test("F1 has a K-specific label and report preserves processor quality data", () => {
  assert.equal(metricLabel("rag.f1", { k: 5 }), "检索 F1 @5");
  const quality = [{ id: "identity", points: [{ k: 5, precision: 0, recall: null, f1: null }] }];
  assert.equal(basicSummary({ outputs: [{ processor: "builtin.basic-summary@1", data: { rag_quality: quality } }] }).rag_quality, quality);
  assert.equal(basicSummary({ outputs: [{ processor: "custom", data: { rag_quality: quality } }] }), undefined);
});
test("data rendering preserves false and zero", () => {
  assert.equal(pretty(false), "false");
  assert.equal(pretty(0), "0");
  assert.equal(pretty(null), "—");
});
test("failed start is never automatically retried", async () => {
  const original = global.fetch;
  let calls = 0;
  global.fetch = async () => {
    calls++;
    throw Error("provider-secret");
  };
  try {
    await assert.rejects(
      () => evaluationApi.start("smoke"),
      (error) =>
        error instanceof ProductError && error.code === "START_UNKNOWN",
    );
    assert.equal(calls, 1);
  } finally {
    global.fetch = original;
  }
});
test("API rejects raw failed-response text and preserves business code", async () => {
  const original = global.fetch;
  global.fetch = async () => ({
    ok: false,
    json: async () => ({ code: "BUNDLE_INVALID", error: "password" }),
  });
  try {
    await assert.rejects(
      () => evaluationApi.import({}),
      (error) => error.code === "BUNDLE_INVALID",
    );
  } finally {
    global.fetch = original;
  }
});
