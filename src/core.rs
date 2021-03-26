use anyhow::Error;

type Id = i64;

pub struct Node {
  pub id: Id,
}

pub struct Thread {}

pub struct Source {}

pub struct Destination {}

pub struct Filter {}

pub struct Repository {}

pub struct User {}

pub struct Team {}

pub struct Namespace {}

pub trait NodeStore {
  fn get(&self, node_id: &Id) -> Result<Node, Error>;
  fn forks(&self, node_id: &Id) -> Result<Vec<Thread>, Error>;
  fn replies(&self, node_id: &Id) -> Result<Vec<Node>, Error>;
  fn fork(&self, source_id: &Id, to: &Namespace) -> Result<Thread, Error>;
}
