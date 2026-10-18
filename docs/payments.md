# Payments

How customers pay, how the server learns a payment succeeded, and what
operators do when money arrives that should not have. The code lives in
`internal/payments` (providers) and `internal/service/payment` (the rules).

## Providers

| ID | Name | Countries | Currencies | Online |
| --- | --- | --- | --- | --- |
| `cash_on_delivery` | Cash on delivery | any | any | no |
| `mpesa` | M-Pesa (Daraja STK push) | KE | KES | yes |
| `paystack` | Paystack (hosted checkout) | NG, GH, ZA, KE, CI | NGN, GHS, ZAR, KES, USD | yes |

Admins enable providers and enter credentials on the admin host at
`/payments`. Secret fields are encrypted with `SECRETS_KEY` before they are
stored and are never shown again. At least one method must stay enabled.
A provider can only be enabled when it supports the store currency.

Adding a provider means implementing `payments.Provider` (and
`WebhookParser` if it pushes events, `RefundProvider` if it can refund) and
registering it in `payments.NewRegistry`. The setup form is generated from the
provider's `Descriptor`.

## Configuration

| Variable | Why payments need it |
| --- | --- |
| `SECRETS_KEY` | 32 random bytes, base64. Encrypts provider credentials. |
| `SECRETS_KEY_PREVIOUS` | Optional. Lets the server read credentials sealed with the old key during rotation. |
| `PUBLIC_BASE_URL` | Origin providers call back to. HTTPS and an allowed host in production. |

In production, `serve` refuses to start when an enabled online provider is
missing credentials, cannot charge the store currency, or has no public URL.

## Paying online

1. Checkout places the order with a `pending` payment, as for cash.
2. The customer is sent to `/payments/order/<id>`, which starts an attempt:
   an STK push to their phone (M-Pesa) or a redirect to Paystack's page.
   Each attempt has a unique reference and, for M-Pesa, a random callback
   token whose SHA-256 is stored.
3. The provider reports back:
   - Paystack: signed webhook at `/payments/webhooks/paystack` (HMAC-SHA512
     with the secret key) and the customer's return to `/payments/return`.
   - M-Pesa: callback at `/payments/callback/mpesa/<token>`. The token
     authenticates it and the request IDs must match the stored attempt.
4. Every success is confirmed with the provider's own status API before the
   payment is captured, and the confirmed amount and currency must match what
   was charged. The status page also polls the provider, at most once per
   reference per interval.

A retry creates a new attempt. Older attempts stay on record and remain
addressable, so a delayed success for an earlier prompt still counts.

## Unpaid orders

An online payment still pending after 30 minutes, with no attempt younger
than that, is expired by the maintenance job: the order is cancelled, stock
is released, and the payment and its open attempts are marked failed.

A customer can also cancel their own order from `/account` while the vendor
has not accepted it and nothing has been captured. This closes any open
attempt the same way.

## Late and duplicate charges

A provider can still confirm a charge after the order was cancelled (late
capture), or a second attempt can succeed after another already paid
(duplicate charge). Neither is dropped. `CapturePayment` records the charge
and opens a row in `payment_exceptions`, and the cancelled order stays out of
the fulfilment queue. The admin dashboard shows how many exceptions need a
decision.

On `/payments`, the exceptions table offers:

| Provider | Action |
| --- | --- |
| Paystack | **Request refund** calls Paystack's refund API once. The row moves to `pending` and **Check status** reads the result. A processed refund marks it `refunded`. |
| M-Pesa | Reverse the transaction in the M-Pesa portal, then **Record refund** with the reversal receipt. |
| any, stuck | A row in `requested`, `pending`, or `review` can be closed with **Mark refunded** and the provider's reference after reconciling it in the provider dashboard. |

A refund request is claimed in the database before Paystack is called. If the
call fails or times out, the row goes to `review` and is never sent again
automatically, so a customer cannot be refunded twice. When a late capture is
refunded, the payment itself becomes `refunded`; a refunded duplicate charge
leaves the original captured payment alone.

Every refund action is written to the audit log.
