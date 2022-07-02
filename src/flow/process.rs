use crate::Result;

use super::{InPort, OutPort, Packet};

pub struct Process {
  pub name: String
}

impl Process {

  pub fn open_in_port(&self, port_name: String) -> Option<InPort> {
    todo!()
  }

  pub fn open_in_array_port(&self, port_name: String) -> Option<Vec<InPort>> {
    todo!()
  }

  pub fn open_out_port(&self, port_name: String) -> Option<OutPort> {
    todo!()
  }

  pub fn open_out_array_port(&self, port_name: String) -> Option<Vec<OutPort>> {
    todo!()
  }

  pub fn send(&self, output: &OutPort, packet: Packet) -> Result<u64> {
    todo!()
  }
}
