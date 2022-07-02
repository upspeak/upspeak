use super::process::Process;

pub trait Component {
  fn setup(self, proc: Process) -> Self;
  fn execute(self, proc: Process);
}

pub trait ComponentMustRun: Component {
  fn must_run(&self) -> bool;
}
