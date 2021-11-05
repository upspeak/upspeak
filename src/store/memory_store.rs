use crate::core::{DataType, Node, NodeCommand, NodeId};
use crate::{Result};
use std::collections::HashMap;

struct NodeIdGenerator(NodeId);

impl NodeIdGenerator {
  fn new() -> NodeId {
    0i64
  }

  fn next(&mut self) -> NodeId {
    self.0 = self.0 + 1i64;
    self.0
  }
}

pub struct MemoryStore {
  nodes: HashMap<NodeId, Node>,
  gen_node_id: NodeIdGenerator,
}

impl NodeCommand for MemoryStore {
  fn create_node(&mut self, node: Node) -> Result<NodeId> {
    unimplemented!()
  }

  fn create_fork(
    &mut self,
    source_node_id: NodeId,
    quoted_data: DataType,
  ) -> Result<NodeId> {
    unimplemented!()
  }

  fn create_child(&mut self, parent_node_id: NodeId, child: Node) -> Result<NodeId> {
    Ok(self.gen_node_id.next())
  }
}
