use crate::Result;

use super::{Packet, Process};

pub struct InPort;

pub struct OutPort;

impl OutPort {
  pub fn send(&self, process: Process, packet: Packet) -> Result<u8> {
    todo!()
  }
}
