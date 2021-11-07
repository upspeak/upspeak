use crate::core::*;
use crate::Result;
use sled;

pub struct LocalStore {
  db: sled::Db,
  nodes_tree: sled::Tree,
  repos_tree: sled::Tree,
  users_tree: sled::Tree
}

impl LocalStore {
  pub fn open(conn_str: String) -> Result<LocalStore> {
    let db = sled::open(conn_str)?;
    let nodes_tree = db.open_tree("nodes")?;
    let repos_tree = db.open_tree("repos")?;
    let users_tree = db.open_tree("users")?;

    Ok(LocalStore {
      db,
      nodes_tree,
      repos_tree,
      users_tree
    })
  }
}

impl NodeCommand for LocalStore {
  fn create_node(&mut self, node: Node) -> Result<NodeId> {
    unimplemented!()
  }
  fn create_fork(&mut self, source_node_id: NodeId, quoted_data: DataType) -> Result<NodeId> {
    unimplemented!()
  }
  fn create_child(&mut self, parent_node_id: NodeId, child: Node) -> Result<NodeId> {
    unimplemented!()
  }
}

impl NodeQuery for LocalStore {
  fn node(&self, node_id: NodeId) -> Result<Node> {
    unimplemented!()
  }
  fn children(&self, node_id: NodeId) -> Result<Vec<Node>> {
    unimplemented!()
  }
  fn forks(&self, node_id: NodeId) -> Result<Vec<Node>> {
    unimplemented!()
  }
  fn forked_from(&self, node_id: NodeId) -> Result<Node> {
    unimplemented!()
  }
  fn replies(&self, node_id: NodeId) -> Result<Vec<Node>> {
    unimplemented!()
  }
  fn in_reply_to(&self, node_id: NodeId) -> Result<Node> {
    unimplemented!()
  }
}

impl UserQuery for LocalStore {
  fn user(&self, username: UserName, hostname: Hostname) -> Result<User> {
    unimplemented!()
  }
}

impl UserCommand for LocalStore {
  fn create_user(&mut self, user: User) -> Result<UserName>{
    unimplemented!()
  }
}
