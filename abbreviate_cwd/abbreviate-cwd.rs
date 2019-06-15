use std::env::current_dir;
use std::error::Error;
use std::path::{PathBuf, Path};
use std::str::Chars;

fn main() {
    let abbreviate_res = abbreviate_to_length();
    match abbreviate_res {
        Ok(abbrv) => println!("{}", abbrv),
        Err(err) => eprintln!("Failure: {}", err)
    }
}

fn abbreviate_to_length() -> Result<String, Box<dyn Error>> {
    let input_path: PathBuf = current_dir()?;
    let user_home: PathBuf = match dirs::home_dir() {
        Some(h) => h,
        None => PathBuf::new()
    };
    let path_from_home = if input_path.starts_with(user_home.as_path()) {
        let path_stripped_home: PathBuf = input_path.strip_prefix(user_home)?.to_path_buf();
        let path_tilda: PathBuf = PathBuf::from("~").join(path_stripped_home);
        path_tilda
    } else {
        input_path.to_path_buf()
    };
    let (count, mut total_length) = count_and_total_length(path_from_home.as_path());
    let mut path: String = String::with_capacity(total_length);
    let mut index: usize = 0;
    for elem in path_from_home.iter() {
        // might be empty string?
        match elem.to_str() {
            None => {
                index += 1;
            }
            Some(e) => {
                if index > 0 {
                    path.push('/');
                }
                index += 1;
                // file system root '/' is recognized as the first element,
                // should not add it because we will add / before each element:
                if e == "/" {
                    // '/' will not be added if we are in FS root,
                    // but we can shortcut it:
                    if count == 1 {
                        path.push('/');
                        break;
                    }
                    continue;
                }
                // do not abbreviate last element:
                if index == count {
                    path.push_str(e);
                } else {
                    // have to abbreviate:
                    let mut chars: Chars = e.chars();
                    match chars.next() {
                        None => continue,
                        Some(c) => {
                            path.push(c);
                            // reduce total length by the element length:
                            total_length -= e.len();
                            // add single char back:
                            total_length += 1;
                            if c == '.' {
                                // need a second char if we had a '.':
                                match chars.next() {
                                    None => continue,
                                    Some(c2) => {
                                        path.push(c2);
                                        total_length += 1;
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    return Ok(path);
}

fn count_and_total_length(path: &Path) -> (usize, usize) {
    let mut count: usize = 0;
    let mut total_length: usize = 0;
    for elem in path.iter() {
        count += 1;
        total_length += 1;
        total_length += elem.len();
    }
    return (count, total_length);
}
