use crate::Result;

pub trait Node {
  type NodeType;

  fn content(&self) -> Self::NodeType;
  fn children<T: Node>(&self) -> Vec<T>;
}

pub trait Source {
  type Config;

  fn receive<T: Node>(&self, config: Self::Config) -> Result<Vec<T>>;
}

pub trait Destination {
  type Config;

  fn send<T: Node>(&self, config: Self::Config, nodes: Vec<T>) -> Result<()>;
}

pub trait Filter {
  type Config;

  fn filter<T: Node>(&self, config: Self::Config, nodes: Vec<T>) -> Result<Vec<T>>;
}

pub trait Action {
  type Input;
  type Output;

  fn run(self, input: Self::Input) -> Result<Self::Output>;
}

pub trait Render<T> {
  fn render(&self) -> Result<T>;
}

pub struct Repository {}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn test_actionable() {
    struct Multiplier(i64);

    impl Action for Multiplier {
      type Input = i64;
      type Output = i64;

      fn run(self, input: Self::Input) -> Result<Self::Output> {
        Ok(self.0 * input)
      }
    }

    let m1 = Multiplier(-50);
    let m2 = Multiplier(25);

    assert_eq!(m1.run(-2).unwrap(), m2.run(4).unwrap());
  }

  #[test]
  fn test_renderer() {
    struct HTMLNode {
      title: String,
      body: String,
    }

    impl Render<String> for HTMLNode {
      fn render(&self) -> Result<String> {
        Ok(format!(
          "<html><head><title>{}</title></head><body>{}</body></html>",
          self.title, self.body
        ))
      }
    }

    impl Render<i64> for HTMLNode {
      fn render(&self) -> Result<i64> {
        Ok(42)
      }
    }

    let h = HTMLNode {
      title: "Hello".to_string(),
      body: "<p><b>World</b></p>".to_string(),
    };

    let s: String = h.render().unwrap();
    let n: i64 = h.render().unwrap();
    assert_eq!(
      s,
      "<html><head><title>Hello</title></head><body><p><b>World</b></p></body></html>"
    );
    assert_eq!(n, 42);
  }
}
