use std::collections::HashMap;

use super::{component::Component, process::Process};

pub struct Network {
  pub name: String,
  procs: HashMap<String, Process>,
  mother: Option<Box<Process>>,
}

impl Network {
  pub fn new(name: String) -> Network {
    Network {
      name,
      procs: HashMap::new(),
      mother: None,
    }
  }

  pub fn new_subnet(name: String, p: Process) -> Network {
    let mut net = Network::new(name);
    net.mother = Some(Box::new(p));
    net
  }
}

impl Network {
  pub fn get_proc(&self, name: &str) -> Option<&Process> {
    self.procs.get(name)
  }

  pub fn set_proc(&mut self, name: String, proc: Process) {
    self.procs.insert(name, proc);
  }

  // TODO: Fix ownership
  pub fn new_process(&self, name: String, component: impl Component) -> Process {
    Process {
      name
    }
  }
}

#[cfg(test)]
mod tests {
  use crate::flow::{Component, Process};

  use super::Network;

  #[test]
  fn test_process() {
    struct test_cmp(String);

    impl Component for test_cmp {
      fn setup(self, proc: Process) -> Self {
        self
      }

      fn execute(self, proc: Process) {
        todo!()
      }
    }

    let net1 = Network::new("net1".to_string());
    let proc1 = net1.new_process("proc1".to_string(), test_cmp("test_cmp1".to_string()));
    let proc2 = net1.new_process("proc2".to_string(), test_cmp("test_cmp2".to_string()));
  }
}
