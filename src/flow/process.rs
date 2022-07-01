use super::network::Network;

pub struct Process {
  gid: u64,
  pub name: String,
  network: Network,

}
