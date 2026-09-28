import Table from "antd/es/table";
import type { RetrievalDocument } from "../../domain/types";

export function RetrievalEvidence({documents, relevance}: {documents: RetrievalDocument[]; relevance?: Record<string,number>}) {
  return <section aria-label="检索排名"><h3>检索排名</h3>
    <Table<RetrievalDocument> size="small" pagination={false} rowKey="id" scroll={{x: 620}}
      dataSource={documents} locale={{emptyText: "没有检索结果"}}
      columns={[
        {title: "排名", width: 64, render: (_value, _record, index) => index + 1},
        {title: "文档 ID", dataIndex: "id", width: 160, render: value => <span style={{overflowWrap:"anywhere"}}>{value}</span>},
        {title: "检索分数", dataIndex: "score", width: 100, render: value => value ?? "—"},
        {title: "参考等级", width: 100, render: (_value, document) => relevance?.[document.id] ?? "—"},
        {title: "文档内容", dataIndex: "text", render: value => <span style={{whiteSpace:"pre-wrap", overflowWrap:"anywhere"}}>{value}</span>},
      ]}/>
  </section>;
}
