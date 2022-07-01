use std::collections::HashMap;

use super::process::Process;

pub struct Network {
  pub name: String,
  procs: HashMap<String, Process>
}
