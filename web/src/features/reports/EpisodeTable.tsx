import Button from "antd/es/button";
import Table from "antd/es/table";
import Tag from "antd/es/tag";
import type { EpisodeResult } from "../../domain/types";
import { StatusBadge } from "../../components/StatusBadge";

export function EpisodeTable({ episodes, onOpen }: {
  episodes: EpisodeResult[];
  onOpen: (id: string) => void;
}) {
  return <Table<EpisodeResult> rowKey="id" size="small" pagination={false}
    dataSource={episodes} scroll={{ x: 680 }} columns={[
      { title: "Episode", dataIndex: "id", width: 200,
        render: id => <span style={{ overflowWrap: "anywhere" }}>{id}</span> },
      { title: "执行状态", dataIndex: "status", width: 110,
        render: status => <StatusBadge status={status} /> },
      { title: "已处理 / 计划轮次", width: 150,
        render: (_value, episode) => `${episode.completed} / ${episode.case_ids.length}` },
      { title: "按顺序查看轮次", render: (_value, episode) => episode.case_ids.map((id,index) => {
        return index < episode.completed ? <Button key={id} type="link" onClick={() => onOpen(id)}
          style={{ height: "auto", whiteSpace: "normal", overflowWrap: "anywhere" }}>
          {id}
        </Button> : <Tag key={id} style={{ whiteSpace: "normal", overflowWrap: "anywhere" }}>{id} · 未执行</Tag>;
      }) },
    ]} />;
}
