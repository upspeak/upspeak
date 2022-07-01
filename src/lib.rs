use std::error;
use std::fmt;
use std::io;
use std::result;

pub mod flow;

#[derive(Debug)]
pub enum Error {
  NotFound,
  Io(io::Error),
}

impl fmt::Display for Error {
  fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
    match *self {
      Error::Io(ref err) => err.fmt(f),
      Error::NotFound => write!(f, "Resource not found"),
    }
  }
}

impl error::Error for Error {}

impl From<io::Error> for Error {
  fn from(err: io::Error) -> Error {
    Error::Io(err)
  }
}

pub type Result<T> = result::Result<T, Error>;
