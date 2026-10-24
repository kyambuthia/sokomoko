//! Narrow browser boundary around OpenMLS. All state returned here is SECRET:
//! callers must persist it locally and must never send it to the application API.
use ::tls_codec::{Deserialize as _, Serialize as _};
use base64::{Engine as _, engine::general_purpose::STANDARD as B64};
use openmls::prelude::*;
use openmls_basic_credential::SignatureKeyPair;
use openmls_rust_crypto::OpenMlsRustCrypto;
use openmls_traits::{OpenMlsProvider, signatures::Signer};
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use wasm_bindgen::prelude::*;

const SUITE: Ciphersuite = Ciphersuite::MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519;
type Result<T> = std::result::Result<T, String>;
fn err<E: std::fmt::Debug>(e: E) -> String {
    format!("MLS operation rejected: {e:?}")
}
fn decode(s: &str) -> Result<Vec<u8>> {
    B64.decode(s).map_err(|_| "Invalid encoding".into())
}

#[derive(Serialize, Deserialize)]
struct State {
    identity: String,
    signer: SignatureKeyPair,
    storage: BTreeMap<String, String>,
}
#[derive(Deserialize)]
struct Request {
    action: String,
    state: Option<State>,
    #[serde(default)]
    identity: String,
    #[serde(default)]
    group: String,
    #[serde(default)]
    message: String,
    #[serde(default)]
    packages: Vec<String>,
    #[serde(default)]
    text: String,
}
#[derive(Serialize)]
struct Member {
    identity: String,
    public_key: String,
}
#[derive(Serialize)]
struct Response {
    state: State,
    public_key: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    message: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    welcome: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    text: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    sender: Option<String>,
    members: Vec<Member>,
}
fn credential(identity: &str, signer: &SignatureKeyPair) -> CredentialWithKey {
    CredentialWithKey {
        credential: BasicCredential::new(identity.as_bytes().to_vec()).into(),
        signature_key: signer.public().into(),
    }
}
fn members(group: &MlsGroup) -> Result<Vec<Member>> {
    group
        .members()
        .map(|m| {
            Ok(Member {
                identity: String::from_utf8(
                    BasicCredential::try_from(m.credential)
                        .map_err(err)?
                        .identity()
                        .to_vec(),
                )
                .map_err(err)?,
                public_key: B64.encode(&m.signature_key),
            })
        })
        .collect()
}
fn run(req: Request) -> Result<Response> {
    let provider = OpenMlsRustCrypto::default();
    let mut state = match req.state {
        Some(s) => s,
        None if req.action == "identity"
            && !req.identity.is_empty()
            && req.identity.len() <= 128 =>
        {
            State {
                identity: req.identity,
                signer: SignatureKeyPair::new(SUITE.signature_algorithm()).map_err(err)?,
                storage: BTreeMap::new(),
            }
        }
        _ => return Err("Missing device state".into()),
    };
    for (k, v) in &state.storage {
        provider
            .storage()
            .values
            .write()
            .map_err(err)?
            .insert(decode(k)?, decode(v)?);
    }
    let mut out = Response {
        public_key: B64.encode(state.signer.public()),
        message: None,
        welcome: None,
        text: None,
        sender: None,
        members: vec![],
        state: State {
            identity: String::new(),
            signer: SignatureKeyPair::new(SUITE.signature_algorithm()).map_err(err)?,
            storage: BTreeMap::new(),
        },
    };
    let group_id = GroupId::from_slice(req.group.as_bytes());
    match req.action.as_str() {
        "identity" => {}
        "sign" => {
            out.message = Some(B64.encode(state.signer.sign(req.text.as_bytes()).map_err(err)?));
        }
        "package" => {
            let bundle = KeyPackage::builder()
                .build(
                    SUITE,
                    &provider,
                    &state.signer,
                    credential(&state.identity, &state.signer),
                )
                .map_err(err)?;
            out.message =
                Some(B64.encode(bundle.key_package().tls_serialize_detached().map_err(err)?));
        }
        "create" => {
            if req.packages.is_empty() || req.packages.len() > 31 || req.group.is_empty() {
                return Err("Invalid group members".into());
            }
            if MlsGroup::load(provider.storage(), &group_id)
                .map_err(err)?
                .is_some()
            {
                return Err("Group already exists".into());
            }
            let config = MlsGroupCreateConfig::builder()
                .ciphersuite(SUITE)
                .use_ratchet_tree_extension(true)
                .build();
            let mut group = MlsGroup::new_with_group_id(
                &provider,
                &state.signer,
                &config,
                group_id,
                credential(&state.identity, &state.signer),
            )
            .map_err(err)?;
            let packages: Vec<KeyPackage> = req
                .packages
                .iter()
                .map(|p| {
                    KeyPackageIn::tls_deserialize_exact(decode(p)?)
                        .map_err(err)?
                        .validate(provider.crypto(), ProtocolVersion::Mls10)
                        .map_err(err)
                })
                .collect::<Result<_>>()?;
            let (_, welcome, _) = group
                .add_members(&provider, &state.signer, &packages)
                .map_err(err)?;
            group.merge_pending_commit(&provider).map_err(err)?;
            out.welcome = Some(B64.encode(welcome.tls_serialize_detached().map_err(err)?));
            out.members = members(&group)?;
        }
        "join" => {
            if MlsGroup::load(provider.storage(), &group_id)
                .map_err(err)?
                .is_some()
            {
                return Err("Group already joined".into());
            }
            let message =
                MlsMessageIn::tls_deserialize_exact(decode(&req.message)?).map_err(err)?;
            let welcome = match message.extract() {
                MlsMessageBodyIn::Welcome(w) => w,
                _ => return Err("Expected Welcome".into()),
            };
            let group = StagedWelcome::new_from_welcome(
                &provider,
                &MlsGroupJoinConfig::default(),
                welcome,
                None,
            )
            .map_err(err)?
            .into_group(&provider)
            .map_err(err)?;
            if group.group_id() != &group_id {
                return Err("Wrong group".into());
            }
            out.members = members(&group)?;
        }
        "send" | "receive" | "update" => {
            let mut group = MlsGroup::load(provider.storage(), &group_id)
                .map_err(err)?
                .ok_or("Group is unavailable")?;
            match req.action.as_str() {
                "send" => {
                    if req.text.is_empty() || req.text.len() > 16000 {
                        return Err("Invalid message length".into());
                    }
                    out.message = Some(
                        B64.encode(
                            group
                                .create_message(&provider, &state.signer, req.text.as_bytes())
                                .map_err(err)?
                                .tls_serialize_detached()
                                .map_err(err)?,
                        ),
                    );
                }
                "update" => {
                    let commit = group
                        .self_update(&provider, &state.signer, LeafNodeParameters::default())
                        .map_err(err)?
                        .into_commit();
                    group.merge_pending_commit(&provider).map_err(err)?;
                    out.message = Some(B64.encode(commit.tls_serialize_detached().map_err(err)?));
                }
                _ => {
                    let message = MlsMessageIn::tls_deserialize_exact(decode(&req.message)?)
                        .map_err(err)?
                        .try_into_protocol_message()
                        .map_err(err)?;
                    let processed = group.process_message(&provider, message).map_err(err)?;
                    out.sender = Some(
                        String::from_utf8(
                            BasicCredential::try_from(processed.credential().clone())
                                .map_err(err)?
                                .identity()
                                .to_vec(),
                        )
                        .map_err(err)?,
                    );
                    match processed.into_content() {
                        ProcessedMessageContent::ApplicationMessage(m) => {
                            out.text = Some(String::from_utf8(m.into_bytes()).map_err(err)?)
                        }
                        ProcessedMessageContent::StagedCommitMessage(commit) => {
                            if commit.add_proposals().next().is_some()
                                || commit.remove_proposals().next().is_some()
                            {
                                return Err(
                                    "Membership changes require a new approved roster".into()
                                );
                            }
                            group.merge_staged_commit(&provider, *commit).map_err(err)?;
                        }
                        _ => return Err("Standalone proposals are unsupported".into()),
                    }
                }
            }
            out.members = members(&group)?;
        }
        _ => return Err("Unknown operation".into()),
    }
    state.storage = provider
        .storage()
        .values
        .read()
        .map_err(err)?
        .iter()
        .map(|(k, v)| (B64.encode(k), B64.encode(v)))
        .collect();
    out.state = state;
    Ok(out)
}

#[wasm_bindgen]
pub fn execute(input: &str) -> std::result::Result<String, JsValue> {
    let request =
        serde_json::from_str(input).map_err(|_| JsValue::from_str("Invalid crypto request"))?;
    let response = run(request).map_err(|e| JsValue::from_str(&e))?;
    serde_json::to_string(&response).map_err(|_| JsValue::from_str("Cannot serialize crypto state"))
}

#[cfg(test)]
mod tests {
    use super::*;
    fn request(
        action: &str,
        state: Option<State>,
        group: &str,
        message: &str,
        text: &str,
        packages: Vec<String>,
    ) -> Request {
        Request {
            action: action.into(),
            state,
            identity: "".into(),
            group: group.into(),
            message: message.into(),
            text: text.into(),
            packages,
        }
    }
    fn identity(id: &str) -> State {
        run(Request {
            identity: id.into(),
            ..request("identity", None, "", "", "", vec![])
        })
        .unwrap()
        .state
    }
    #[test]
    fn offline_join_encrypt_reply_update_and_tamper() {
        let a = identity("1:alice");
        let b = identity("2:bob");
        let outsider = identity("3:eve");
        let kp = run(request("package", Some(b), "", "", "", vec![])).unwrap();
        let created = run(request(
            "create",
            Some(a),
            "conversation:1",
            "",
            "",
            vec![kp.message.unwrap()],
        ))
        .unwrap();
        assert_eq!(created.members.len(), 2);
        let welcome = created.welcome.clone().unwrap();
        assert!(
            run(request(
                "join",
                Some(outsider),
                "conversation:1",
                &welcome,
                "",
                vec![]
            ))
            .is_err()
        );
        let joined = run(request(
            "join",
            Some(kp.state),
            "conversation:1",
            &welcome,
            "",
            vec![],
        ))
        .unwrap();
        let sent = run(request(
            "send",
            Some(created.state),
            "conversation:1",
            "",
            "private text",
            vec![],
        ))
        .unwrap();
        let cipher = sent.message.unwrap();
        assert!(!String::from_utf8_lossy(&decode(&cipher).unwrap()).contains("private text"));
        let mut corrupt = decode(&cipher).unwrap();
        let n = corrupt.len();
        corrupt[n - 1] ^= 1;
        let serialized = serde_json::to_string(&joined.state).unwrap();
        assert!(
            run(request(
                "receive",
                Some(serde_json::from_str(&serialized).unwrap()),
                "conversation:1",
                &B64.encode(corrupt),
                "",
                vec![]
            ))
            .is_err()
        );
        let received = run(request(
            "receive",
            Some(joined.state),
            "conversation:1",
            &cipher,
            "",
            vec![],
        ))
        .unwrap();
        assert_eq!(received.text.as_deref(), Some("private text"));
        assert_eq!(received.sender.as_deref(), Some("1:alice"));
        // Replaying a consumed generation fails after restoring persisted state.
        let saved = serde_json::to_string(&received.state).unwrap();
        assert!(
            run(request(
                "receive",
                Some(serde_json::from_str(&saved).unwrap()),
                "conversation:1",
                &cipher,
                "",
                vec![]
            ))
            .is_err()
        );
        let reply = run(request(
            "send",
            Some(received.state),
            "conversation:1",
            "",
            "reply",
            vec![],
        ))
        .unwrap();
        let received = run(request(
            "receive",
            Some(sent.state),
            "conversation:1",
            &reply.message.unwrap(),
            "",
            vec![],
        ))
        .unwrap();
        assert_eq!(received.text.as_deref(), Some("reply"));
        let updated = run(request(
            "update",
            Some(received.state),
            "conversation:1",
            "",
            "",
            vec![],
        ))
        .unwrap();
        let bob = run(request(
            "receive",
            Some(reply.state),
            "conversation:1",
            &updated.message.unwrap(),
            "",
            vec![],
        ))
        .unwrap();
        let sent = run(request(
            "send",
            Some(updated.state),
            "conversation:1",
            "",
            "after update",
            vec![],
        ))
        .unwrap();
        let bob = run(request(
            "receive",
            Some(bob.state),
            "conversation:1",
            &sent.message.unwrap(),
            "",
            vec![],
        ))
        .unwrap();
        assert_eq!(bob.text.as_deref(), Some("after update"));
    }
}
