use crate::{Result};
use std::fmt;

pub type UserName = String;
pub type Hostname = String;

#[derive(Debug)]
pub enum User {
  Anonymous,
  Local(UserName),
  Remote(UserName, Hostname)
}

impl fmt::Display for User {
  fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
    match *self {
      User::Anonymous => write!(f, "@{}:local", "anonymous"),
      User::Local(ref username) => write!(f, "@{}:local", username),
      User::Remote(ref username, ref hostname) => write!(f, "@{}:{}", username, hostname),
    }
  }
}

#[derive(Debug)]
pub enum DataType {
  Empty,
  Text(String),
  Markdown(String),
  Binary(Vec<u8>),
}

pub type NodeId = i64;

#[derive(Debug)]
pub struct Meta {
  pub created_at: i64,
  pub created_by: User,
  pub updated_at: Option<i64>,
  pub updated_by: Option<User>,
}

#[derive(Debug)]
pub struct Relations {
  pub children: Vec<NodeId>,
  pub forks: Vec<NodeId>,
  pub replies: Vec<NodeId>,
  pub in_reply_to: Option<NodeId>,
  pub root_node: Option<NodeId>,
}

#[derive(Debug)]
pub struct Node {
  pub id: NodeId,
  pub title: Option<String>,
  pub body: DataType,
  pub meta: Meta,
  pub relations: Relations,
}

impl Node {
  pub fn is_leaf(&self) -> bool {
    self.relations.children.len() < 1
  }
}

pub type RepoId = i64;

#[derive(Debug)]
pub enum Item {
  Node(Node),
  Thread(Node),
}

pub struct Repo {
  pub id: RepoId,
  pub path: String,
  pub title: String,
  pub description: String,
  pub items: Vec<Item>,
  pub meta: Meta,
}

pub trait NodeQuery {
  fn node(&self, node_id: NodeId) -> Result<Node>;
  fn children(&self, node_id: NodeId) -> Result<Vec<Node>>;
  fn forks(&self, node_id: NodeId) -> Result<Vec<Node>>;
  fn forked_from(&self, node_id: NodeId) -> Result<Node>;
  fn replies(&self, node_id: NodeId) -> Result<Vec<Node>>;
  fn in_reply_to(&self, node_id: NodeId) -> Result<Node>;
}

pub trait NodeCommand {
  fn create_node(&mut self, node: Node) -> Result<NodeId>;
  fn create_fork(&mut self, source_node_id: NodeId, quoted_data: DataType) -> Result<NodeId>;
  fn create_child(&mut self, parent_node_id: NodeId, child: Node) -> Result<NodeId>;
}

pub trait RepoQuery {
  fn repo(&self, repo_id: RepoId) -> Result<Repo>;
}

pub trait RepoCommand {
  fn create_repo(&mut self, repo: Repo) -> Result<RepoId>;
  fn create_item(&mut self, repo_id: RepoId, item: Item) -> Result<NodeId>;
}

pub trait UserQuery {
  fn user(&self, username: UserName, hostname: Hostname) -> Result<User>;

  fn local_user(&self, username: UserName) -> Result<User> {
    self.user(username, "local".to_string())
  }
}

pub trait UserCommand {
  fn create_user(&mut self, user: User) -> Result<UserName>;
}
